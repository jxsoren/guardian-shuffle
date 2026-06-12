package poller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/bungie"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

// fakeAPI returns a predetermined sequence of activity hashes.
type fakeAPI struct {
	hashes []uint32
	idx    int
	err    error
}

func (f *fakeAPI) GetProfile(context.Context, string, int64, string) (*bungie.ProfileResponse, error) {
	return nil, nil
}
func (f *fakeAPI) EquipItem(context.Context, string, string, string, int64) error { return nil }
func (f *fakeAPI) GetCharacterActivities(_ context.Context, _ string, _ int64, _, _ string) (uint32, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.idx >= len(f.hashes) {
		return 0, errors.New("fakeAPI: no more hashes")
	}
	h := f.hashes[f.idx]
	f.idx++
	return h, nil
}

type stubCycler struct{ calls int }

func (s *stubCycler) CycleUser(context.Context, int64, time.Time) error {
	s.calls++
	return nil
}

func newPoller(api *fakeAPI, cycler *stubCycler) *userPoller {
	return &userPoller{
		userID: 1,
		user: store.User{
			ID:                 1,
			BungieMembershipID: "m1",
			MembershipType:     3,
			PrimaryCharacterID: "c1",
		},
		st:  store.NewMemory(),
		api: api,
		getToken: func(context.Context, int64, time.Time) (string, error) {
			return "tok", nil
		},
		cycler: cycler,
	}
}

func TestPoll_UnknownToOrbit_NoCycle(t *testing.T) {
	api := &fakeAPI{hashes: []uint32{0}}
	cycler := &stubCycler{}
	up := newPoller(api, cycler)
	state := stateUnknown

	_, stop := up.poll(context.Background(), &state)

	if stop {
		t.Fatal("should not stop")
	}
	if state != stateInOrbit {
		t.Fatalf("expected InOrbit, got %v", state)
	}
	if cycler.calls != 0 {
		t.Fatalf("expected no cycle, got %d", cycler.calls)
	}
}

func TestPoll_UnknownToActivity_NoCycle(t *testing.T) {
	api := &fakeAPI{hashes: []uint32{999}}
	cycler := &stubCycler{}
	up := newPoller(api, cycler)
	state := stateUnknown

	_, stop := up.poll(context.Background(), &state)

	if stop {
		t.Fatal("should not stop")
	}
	if state != stateInActivity {
		t.Fatalf("expected InActivity, got %v", state)
	}
	if cycler.calls != 0 {
		t.Fatalf("expected no cycle, got %d", cycler.calls)
	}
}

func TestPoll_ActivityToOrbit_CyclesFired(t *testing.T) {
	api := &fakeAPI{hashes: []uint32{999, 0}}
	cycler := &stubCycler{}
	up := newPoller(api, cycler)
	state := stateUnknown

	up.poll(context.Background(), &state)      // Unknown → InActivity
	_, stop := up.poll(context.Background(), &state) // InActivity → InOrbit

	if stop {
		t.Fatal("should not stop")
	}
	if state != stateInOrbit {
		t.Fatalf("expected InOrbit, got %v", state)
	}
	if cycler.calls != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycler.calls)
	}
}

func TestPoll_OrbitToOrbit_NoCycle(t *testing.T) {
	api := &fakeAPI{hashes: []uint32{0, 0}}
	cycler := &stubCycler{}
	up := newPoller(api, cycler)
	state := stateUnknown

	up.poll(context.Background(), &state)
	up.poll(context.Background(), &state)

	if cycler.calls != 0 {
		t.Fatalf("expected no cycle, got %d", cycler.calls)
	}
}

func TestPoll_ActivityToActivityToOrbit_OneCycle(t *testing.T) {
	api := &fakeAPI{hashes: []uint32{999, 888, 0}}
	cycler := &stubCycler{}
	up := newPoller(api, cycler)
	state := stateUnknown

	up.poll(context.Background(), &state)
	up.poll(context.Background(), &state)
	up.poll(context.Background(), &state)

	if cycler.calls != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycler.calls)
	}
}

func TestPoll_FastIntervalInActivity_SlowInOrbit(t *testing.T) {
	api := &fakeAPI{hashes: []uint32{999, 0}}
	up := newPoller(api, &stubCycler{})
	state := stateUnknown

	interval1, _ := up.poll(context.Background(), &state)
	interval2, _ := up.poll(context.Background(), &state)

	if interval1 != fastInterval {
		t.Fatalf("expected fastInterval in activity, got %v", interval1)
	}
	if interval2 != slowInterval {
		t.Fatalf("expected slowInterval in orbit, got %v", interval2)
	}
}

func TestPoll_ReauthRequired_StopsPoller(t *testing.T) {
	// reauthErr is declared in user_poller.go as auth.ErrReauthRequired.
	api := &fakeAPI{err: reauthErr}
	up := newPoller(api, &stubCycler{})
	state := stateUnknown

	_, stop := up.poll(context.Background(), &state)

	if !stop {
		t.Fatal("expected stop=true on ErrReauthRequired")
	}
}

func TestPoll_NoCharacterID_SlowIntervalNoCycle(t *testing.T) {
	api := &fakeAPI{hashes: []uint32{999}}
	cycler := &stubCycler{}
	up := newPoller(api, cycler)
	up.user.PrimaryCharacterID = ""
	state := stateUnknown

	interval, stop := up.poll(context.Background(), &state)

	if stop {
		t.Fatal("should not stop")
	}
	if interval != slowInterval {
		t.Fatalf("expected slowInterval, got %v", interval)
	}
	if api.idx != 0 {
		t.Fatal("should not call API when no character ID")
	}
}

func TestPoll_ReauthFromToken_StopsPoller(t *testing.T) {
	up := newPoller(&fakeAPI{hashes: []uint32{999}}, &stubCycler{})
	up.getToken = func(context.Context, int64, time.Time) (string, error) {
		return "", reauthErr
	}
	state := stateUnknown

	_, stop := up.poll(context.Background(), &state)

	if !stop {
		t.Fatal("expected stop=true on ErrReauthRequired from getToken")
	}
}

func TestPoll_PersistsActivityState(t *testing.T) {
	api := &fakeAPI{hashes: []uint32{999}}
	up := newPoller(api, &stubCycler{})
	state := stateUnknown

	up.poll(context.Background(), &state)

	saved, err := up.st.GetActivityState(context.Background(), up.userID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ActivityHash != 999 || saved.CharID != "c1" {
		t.Fatalf("unexpected saved state: %+v", saved)
	}
}
