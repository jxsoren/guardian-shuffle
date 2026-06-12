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
	if _, err := pg.db.Exec("TRUNCATE users, tokens, settings, swap_history RESTART IDENTITY CASCADE"); err != nil {
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
