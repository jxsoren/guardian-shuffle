package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/store"
)

type recordingCycler struct {
	mu     sync.Mutex
	called []int64
}

func (r *recordingCycler) CycleUser(_ context.Context, userID int64, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.called = append(r.called, userID)
	return nil
}

func TestRunOnce_CyclesDueUsers(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	id, _ := st.UpsertUser(ctx, store.User{BungieMembershipID: "m1"})
	_ = st.SaveSettings(ctx, store.Settings{UserID: id, Enabled: true, TriggerMode: "scheduled", IntervalSeconds: 60})

	rc := &recordingCycler{}
	s := New(st, rc)
	if err := s.RunOnce(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(rc.called) != 1 || rc.called[0] != id {
		t.Fatalf("expected user %d cycled, got %v", id, rc.called)
	}
}
