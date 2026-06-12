package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// compile-time assertions that both implementations satisfy Store.
var _ Store = (*Postgres)(nil)
var _ Store = (*Memory)(nil)

// Postgres is a Store backed by a PostgreSQL database.
type Postgres struct{ db *sql.DB }

func NewPostgres(ctx context.Context, url string) (*Postgres, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return &Postgres{db: db}, nil
}

func (p *Postgres) Close() error { return p.db.Close() }

func (p *Postgres) Migrate(ctx context.Context) error {
	b, err := migrationsFS.ReadFile("migrations/0001_init.sql")
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, string(b))
	return err
}

func (p *Postgres) UpsertUser(ctx context.Context, u User) (int64, error) {
	var id int64
	err := p.db.QueryRowContext(ctx, `
		INSERT INTO users (bungie_membership_id, membership_type, primary_character_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (bungie_membership_id) DO UPDATE
		  SET membership_type = EXCLUDED.membership_type,
		      primary_character_id = EXCLUDED.primary_character_id
		RETURNING id`,
		u.BungieMembershipID, u.MembershipType, u.PrimaryCharacterID).Scan(&id)
	return id, err
}

func (p *Postgres) GetUser(ctx context.Context, id int64) (User, error) {
	var u User
	err := p.db.QueryRowContext(ctx,
		`SELECT id, bungie_membership_id, membership_type, primary_character_id FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.BungieMembershipID, &u.MembershipType, &u.PrimaryCharacterID)
	if err != nil {
		return User{}, fmt.Errorf("get user %d: %w", id, err)
	}
	return u, nil
}

func (p *Postgres) SaveTokens(ctx context.Context, t Tokens) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO tokens (user_id, access_token_enc, refresh_token_enc, access_expires_at, refresh_expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE
		  SET access_token_enc = EXCLUDED.access_token_enc,
		      refresh_token_enc = EXCLUDED.refresh_token_enc,
		      access_expires_at = EXCLUDED.access_expires_at,
		      refresh_expires_at = EXCLUDED.refresh_expires_at`,
		t.UserID, t.AccessTokenEnc, t.RefreshTokenEnc, t.AccessExpiresAt, t.RefreshExpiresAt)
	return err
}

func (p *Postgres) GetTokens(ctx context.Context, userID int64) (Tokens, error) {
	var t Tokens
	err := p.db.QueryRowContext(ctx,
		`SELECT user_id, access_token_enc, refresh_token_enc, access_expires_at, refresh_expires_at
		 FROM tokens WHERE user_id = $1`, userID).
		Scan(&t.UserID, &t.AccessTokenEnc, &t.RefreshTokenEnc, &t.AccessExpiresAt, &t.RefreshExpiresAt)
	if err != nil {
		return Tokens{}, fmt.Errorf("get tokens %d: %w", userID, err)
	}
	return t, nil
}

func (p *Postgres) SaveSettings(ctx context.Context, s Settings) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO settings (user_id, enabled, trigger_mode, interval_seconds, last_cycled_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE
		  SET enabled = EXCLUDED.enabled,
		      trigger_mode = EXCLUDED.trigger_mode,
		      interval_seconds = EXCLUDED.interval_seconds,
		      last_cycled_at = EXCLUDED.last_cycled_at`,
		s.UserID, s.Enabled, s.TriggerMode, s.IntervalSeconds, s.LastCycledAt)
	return err
}

func (p *Postgres) GetSettings(ctx context.Context, userID int64) (Settings, error) {
	s := Settings{UserID: userID, TriggerMode: "manual"}
	err := p.db.QueryRowContext(ctx,
		`SELECT enabled, trigger_mode, interval_seconds, last_cycled_at FROM settings WHERE user_id = $1`, userID).
		Scan(&s.Enabled, &s.TriggerMode, &s.IntervalSeconds, &s.LastCycledAt)
	if err == sql.ErrNoRows {
		return s, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return s, nil
}

func (p *Postgres) DueUsers(ctx context.Context, now time.Time) ([]int64, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT user_id FROM settings
		WHERE enabled = true AND trigger_mode = 'scheduled' AND interval_seconds > 0
		  AND (last_cycled_at IS NULL OR last_cycled_at + (interval_seconds * interval '1 second') <= $1)`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (p *Postgres) RecordSwap(ctx context.Context, userID int64, fromHash, toHash uint32, status string) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO swap_history (user_id, from_hash, to_hash, status) VALUES ($1, $2, $3, $4)`,
		userID, int64(fromHash), int64(toHash), status)
	return err
}
