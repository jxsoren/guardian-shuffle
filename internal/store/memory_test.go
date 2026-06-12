package store

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_UserAndSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()

	id, err := s.UpsertUser(ctx, User{BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := s.UpsertUser(ctx, User{BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c2"})
	if id != id2 {
		t.Fatalf("expected stable id, got %d then %d", id, id2)
	}

	if err := s.SaveSettings(ctx, Settings{UserID: id, Enabled: true, TriggerMode: "scheduled", IntervalSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSettings(ctx, id)
	if err != nil || got.IntervalSeconds != 60 || !got.Enabled {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestMemoryStore_DueUsers(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	id, _ := s.UpsertUser(ctx, User{BungieMembershipID: "m1"})
	old := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	_ = s.SaveSettings(ctx, Settings{UserID: id, Enabled: true, TriggerMode: "scheduled", IntervalSeconds: 60, LastCycledAt: &old})

	due, err := s.DueUsers(ctx, time.Date(2026, 6, 1, 0, 2, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0] != id {
		t.Fatalf("expected user due, got %v", due)
	}

	recent := time.Date(2026, 6, 1, 0, 1, 59, 0, time.UTC)
	_ = s.SaveSettings(ctx, Settings{UserID: id, Enabled: true, TriggerMode: "scheduled", IntervalSeconds: 60, LastCycledAt: &recent})
	due, _ = s.DueUsers(ctx, time.Date(2026, 6, 1, 0, 2, 0, 0, time.UTC))
	if len(due) != 0 {
		t.Fatalf("expected none due, got %v", due)
	}
}
