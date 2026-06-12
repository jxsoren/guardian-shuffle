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

func TestMemoryStore_DueUsers_ExactDeadline(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	id, _ := s.UpsertUser(ctx, User{BungieMembershipID: "m1"})
	last := time.Date(2026, 6, 1, 0, 1, 0, 0, time.UTC)
	_ = s.SaveSettings(ctx, Settings{UserID: id, Enabled: true, TriggerMode: "scheduled", IntervalSeconds: 60, LastCycledAt: &last})

	// Querying at exactly last+interval must count as due.
	due, err := s.DueUsers(ctx, last.Add(60*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0] != id {
		t.Fatalf("expected user due exactly at deadline, got %v", due)
	}
}

func TestMemoryStore_ActivityState_MissingReturnsZero(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	got, err := s.GetActivityState(ctx, 99)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != 0 {
		t.Fatalf("expected zero-value for missing user, got %+v", got)
	}
}

func TestMemoryStore_ActivityState_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	id, _ := s.UpsertUser(ctx, User{BungieMembershipID: "m1"})

	state := ActivityState{UserID: id, CharID: "c1", ActivityHash: 12345, UpdatedAt: time.Now().Truncate(time.Second)}
	if err := s.SaveActivityState(ctx, state); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetActivityState(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != id || got.CharID != "c1" || got.ActivityHash != 12345 {
		t.Fatalf("unexpected state: %+v", got)
	}
}

func TestMemoryStore_EventModeUsers(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()

	id1, _ := s.UpsertUser(ctx, User{BungieMembershipID: "m1"})
	id2, _ := s.UpsertUser(ctx, User{BungieMembershipID: "m2"})
	id3, _ := s.UpsertUser(ctx, User{BungieMembershipID: "m3"})

	_ = s.SaveSettings(ctx, Settings{UserID: id1, Enabled: true, TriggerMode: "event"})
	_ = s.SaveSettings(ctx, Settings{UserID: id2, Enabled: false, TriggerMode: "event"}) // disabled
	_ = s.SaveSettings(ctx, Settings{UserID: id3, Enabled: true, TriggerMode: "scheduled"}) // wrong mode

	users, err := s.EventModeUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != id1 {
		t.Fatalf("expected only user %d, got %+v", id1, users)
	}
}
