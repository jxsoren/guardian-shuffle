package poller

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/auth"
	"github.com/jsorensen/guardian_shuffle/internal/bungie"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

const (
	fastInterval = 30 * time.Second
	slowInterval = 120 * time.Second
)

type pollState int

const (
	stateUnknown    pollState = iota
	stateInActivity
	stateInOrbit
)

// reauthErr is the sentinel returned by the token manager when re-authentication is required.
var reauthErr = auth.ErrReauthRequired

// Cycler is the surface of swap.Engine needed by the poller.
type Cycler interface {
	CycleUser(ctx context.Context, userID int64, now time.Time) error
}

type userPoller struct {
	userID   int64
	user     store.User
	st       store.Store
	api      bungie.API
	getToken func(context.Context, int64, time.Time) (string, error)
	cycler   Cycler
}

// run drives the per-user poll loop. Blocks until ctx is cancelled or reauth is required.
func (up *userPoller) run(ctx context.Context) {
	as, _ := up.st.GetActivityState(ctx, up.userID)
	state := stateUnknown
	if as.UserID != 0 {
		if as.ActivityHash != 0 {
			state = stateInActivity
		} else {
			state = stateInOrbit
		}
	}

	interval := slowInterval
	if state == stateInActivity {
		interval = fastInterval
	}

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			next, stop := up.poll(ctx, &state)
			if stop {
				return
			}
			timer.Reset(next)
		}
	}
}

// poll makes one activity check, transitions the state machine, and returns
// (nextInterval, shouldStop). shouldStop is true only on ErrReauthRequired.
func (up *userPoller) poll(ctx context.Context, state *pollState) (time.Duration, bool) {
	if up.user.PrimaryCharacterID == "" {
		log.Printf("poller: user %d: no primary character ID, skipping", up.userID)
		return slowInterval, false
	}

	token, err := up.getToken(ctx, up.userID, time.Now())
	if errors.Is(err, reauthErr) {
		log.Printf("poller: user %d: re-auth required, stopping", up.userID)
		return 0, true
	}
	if err != nil {
		log.Printf("poller: user %d: get token: %v", up.userID, err)
		return slowInterval, false
	}

	hash, err := up.api.GetCharacterActivities(ctx, token, up.user.MembershipType, up.user.BungieMembershipID, up.user.PrimaryCharacterID)
	if err != nil {
		if errors.Is(err, reauthErr) {
			log.Printf("poller: user %d: re-auth required from activities call, stopping", up.userID)
			return 0, true
		}
		log.Printf("poller: user %d: get activities: %v", up.userID, err)
		return slowInterval, false
	}

	prev := *state
	if hash == 0 {
		*state = stateInOrbit
	} else {
		*state = stateInActivity
	}

	if prev == stateInActivity && *state == stateInOrbit {
		if err := up.cycler.CycleUser(ctx, up.userID, time.Now()); err != nil {
			log.Printf("poller: user %d: cycle: %v", up.userID, err)
		}
	}

	if err := up.st.SaveActivityState(ctx, store.ActivityState{
		UserID:       up.userID,
		CharID:       up.user.PrimaryCharacterID,
		ActivityHash: hash,
		UpdatedAt:    time.Now(),
	}); err != nil {
		log.Printf("poller: user %d: save activity state: %v", up.userID, err)
	}

	if *state == stateInActivity {
		return fastInterval, false
	}
	return slowInterval, false
}
