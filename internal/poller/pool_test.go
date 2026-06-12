package poller

import (
	"context"
	"testing"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/bungie"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

// fakeStore wraps store.Memory and overrides EventModeUsers.
type fakeStore struct {
	*store.Memory
	eventUsers []store.User
}

func (f *fakeStore) EventModeUsers(_ context.Context) ([]store.User, error) {
	return f.eventUsers, nil
}

func newFakeStore(users []store.User) *fakeStore {
	return &fakeStore{Memory: store.NewMemory(), eventUsers: users}
}

// blockingAPI blocks GetCharacterActivities until ctx is cancelled, keeping goroutines alive.
type blockingAPI struct{}

func (b *blockingAPI) GetProfile(context.Context, string, int64, string) (*bungie.ProfileResponse, error) {
	return nil, nil
}
func (b *blockingAPI) EquipItem(context.Context, string, string, string, int64) error { return nil }
func (b *blockingAPI) GetCharacterActivities(ctx context.Context, _ string, _ int64, _, _ string) (uint32, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

func newTestPool(fst *fakeStore) *Pool {
	return NewPool(fst, &blockingAPI{},
		func(context.Context, int64, time.Time) (string, error) { return "tok", nil },
		&stubCycler{})
}

func TestPool_ScanStartsGoroutineForEventUser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	u := store.User{ID: 1, BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c1"}
	fst := newFakeStore([]store.User{u})
	p := newTestPool(fst)

	p.scan(ctx)

	p.mu.Lock()
	count := len(p.running)
	p.mu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 running goroutine, got %d", count)
	}
}

func TestPool_ScanStopsGoroutineWhenUserLeavesEventMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	u := store.User{ID: 1, BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c1"}
	fst := newFakeStore([]store.User{u})
	p := newTestPool(fst)

	p.scan(ctx) // start goroutine

	fst.eventUsers = nil
	p.scan(ctx) // should cancel it

	p.mu.Lock()
	count := len(p.running)
	p.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 running goroutines after user left event mode, got %d", count)
	}
}

func TestPool_ScanDoesNotDuplicateGoroutines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	u := store.User{ID: 1, BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c1"}
	fst := newFakeStore([]store.User{u})
	p := newTestPool(fst)

	p.scan(ctx)
	p.scan(ctx)

	p.mu.Lock()
	count := len(p.running)
	p.mu.Unlock()

	if count != 1 {
		t.Fatalf("expected exactly 1 goroutine after two scans, got %d", count)
	}
}
