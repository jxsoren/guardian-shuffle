package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func newTestPostgres(t *testing.T) *Postgres {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	pg, err := NewPostgres(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pg.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pg.db.Exec("TRUNCATE users, tokens, settings, swap_history, activity_states RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pg
}

func TestPostgres_RoundTrip(t *testing.T) {
	ctx := context.Background()
	pg := newTestPostgres(t)
	defer pg.Close()

	id, err := pg.UpsertUser(ctx, User{BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.SaveTokens(ctx, Tokens{
		UserID: id, AccessTokenEnc: []byte("a"), RefreshTokenEnc: []byte("r"),
		AccessExpiresAt: time.Now().Add(time.Hour), RefreshExpiresAt: time.Now().Add(90 * 24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	tk, err := pg.GetTokens(ctx, id)
	if err != nil || string(tk.AccessTokenEnc) != "a" {
		t.Fatalf("got %+v err %v", tk, err)
	}

	old := time.Now().Add(-time.Hour)
	_ = pg.SaveSettings(ctx, Settings{UserID: id, Enabled: true, TriggerMode: "scheduled", IntervalSeconds: 60, LastCycledAt: &old})
	due, err := pg.DueUsers(ctx, time.Now())
	if err != nil || len(due) != 1 || due[0] != id {
		t.Fatalf("due=%v err=%v", due, err)
	}
}

func TestPostgres_ActivityState(t *testing.T) {
	ctx := context.Background()
	pg := newTestPostgres(t)
	defer pg.Close()

	id, err := pg.UpsertUser(ctx, User{BungieMembershipID: "act1", MembershipType: 3, PrimaryCharacterID: "c1"})
	if err != nil {
		t.Fatal(err)
	}

	// Missing state returns zero-value.
	got, err := pg.GetActivityState(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != 0 {
		t.Fatalf("expected zero-value for missing state, got %+v", got)
	}

	// Save and retrieve.
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := pg.SaveActivityState(ctx, ActivityState{UserID: id, CharID: "c1", ActivityHash: 42, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	got, err = pg.GetActivityState(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != id || got.CharID != "c1" || got.ActivityHash != 42 {
		t.Fatalf("unexpected state: %+v", got)
	}
}

func TestPostgres_EventModeUsers(t *testing.T) {
	ctx := context.Background()
	pg := newTestPostgres(t)
	defer pg.Close()

	id1, _ := pg.UpsertUser(ctx, User{BungieMembershipID: "ev1", MembershipType: 3, PrimaryCharacterID: "c1"})
	id2, _ := pg.UpsertUser(ctx, User{BungieMembershipID: "ev2", MembershipType: 3, PrimaryCharacterID: "c2"})

	_ = pg.SaveSettings(ctx, Settings{UserID: id1, Enabled: true, TriggerMode: "event"})
	_ = pg.SaveSettings(ctx, Settings{UserID: id2, Enabled: false, TriggerMode: "event"})

	users, err := pg.EventModeUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != id1 {
		t.Fatalf("expected 1 event user (id=%d), got %+v", id1, users)
	}
}
