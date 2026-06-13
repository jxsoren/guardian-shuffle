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
	profile    *bungie.ProfileResponse
	profileErr error
	hashes     []uint32
	idx        int
	err        error
	gotCharID  string // captures the charID passed to GetCharacterActivities
}

func (f *fakeAPI) GetProfile(context.Context, string, int64, string) (*bungie.ProfileResponse, error) {
	return f.profile, f.profileErr
}
func (f *fakeAPI) EquipItem(context.Context, string, string, string, int64) error { return nil }
func (f *fakeAPI) GetCharacterActivities(_ context.Context, _ string, _ int64, _, charID string) (uint32, error) {
	f.gotCharID = charID
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

// profileWithChar builds a profile whose active (most-recently-played) character is charID.
func profileWithChar(charID string) *bungie.ProfileResponse {
	p := &bungie.ProfileResponse{}
	p.Response.Characters.Data = map[string]bungie.Character{
		charID: {CharacterID: charID, DateLastPlayed: "2024-01-01T00:00:00Z"},
	}
	return p
}

type stubCycler struct{ calls int }

func (s *stubCycler) CycleUser(context.Context, int64, time.Time) error {
	s.calls++
	return nil
}

func newPoller(api *fakeAPI, cycler *stubCycler) *userPoller {
	if api.profile == nil {
		api.profile = profileWithChar("c1")
	}
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

func TestPoll_DerivesActiveCharacterFromProfile(t *testing.T) {
	// Real users have an empty stored PrimaryCharacterID; the poller must still
	// poll the active character it derives from the profile.
	api := &fakeAPI{profile: profileWithChar("c1"), hashes: []uint32{999}}
	cycler := &stubCycler{}
	up := newPoller(api, cycler)
	up.user.PrimaryCharacterID = ""

	state := stateUnknown
	_, stop := up.poll(context.Background(), &state)

	if stop {
		t.Fatal("should not stop")
	}
	if state != stateInActivity {
		t.Fatalf("expected InActivity from derived-char poll, got %v", state)
	}
	if api.gotCharID != "c1" {
		t.Fatalf("expected activities polled for derived char c1, got %q", api.gotCharID)
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

func TestPoll_NoCharacterInProfile_SlowIntervalNoCycle(t *testing.T) {
	// Profile has no characters -> ActiveCharacterID errors -> skip, don't poll activities.
	api := &fakeAPI{profile: &bungie.ProfileResponse{}, hashes: []uint32{999}}
	cycler := &stubCycler{}
	up := newPoller(api, cycler)
	state := stateUnknown

	interval, stop := up.poll(context.Background(), &state)

	if stop {
		t.Fatal("should not stop")
	}
	if interval != slowInterval {
		t.Fatalf("expected slowInterval, got %v", interval)
	}
	if api.idx != 0 {
		t.Fatal("should not poll activities when profile has no character")
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
