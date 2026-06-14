package swap

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/bungie"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

// TokenFunc returns a valid access token for a user (the TokenManager provides this).
type TokenFunc func(ctx context.Context, userID int64, now time.Time) (string, error)

// EmblemSetFunc returns the cached set of emblem item hashes (from the manifest).
type EmblemSetFunc func() map[uint32]bool

// RandFunc returns a *rand.Rand to use for selection (injected for deterministic tests).
type RandFunc func() *rand.Rand

type Engine struct {
	api       bungie.API
	store     store.Store
	token     TokenFunc
	emblemSet EmblemSetFunc
	rng       RandFunc
}

func NewEngine(api bungie.API, st store.Store, token TokenFunc, emblemSet EmblemSetFunc, rng RandFunc) *Engine {
	return &Engine{api: api, store: st, token: token, emblemSet: emblemSet, rng: rng}
}

// ErrNothingToCycle means there were fewer than 2 emblems, so no swap was possible.
var ErrNothingToCycle = fmt.Errorf("no alternate emblem available to cycle to")

// CycleUser performs one emblem swap for the user. It records the outcome and
// updates LastCycledAt on success.
func (e *Engine) CycleUser(ctx context.Context, userID int64, now time.Time) error {
	u, err := e.store.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	token, err := e.token(ctx, userID, now)
	if err != nil {
		return err
	}
	profile, err := e.api.GetProfile(ctx, token, u.MembershipType, u.BungieMembershipID)
	if err != nil {
		return err
	}
	charID, err := bungie.ActiveCharacterID(profile)
	if err != nil {
		return err
	}
	equipped, _ := bungie.EquippedEmblem(profile, charID)
	pool := bungie.EmblemPool(profile, charID, e.emblemSet())
	pick, ok := PickRandom(pool, e.rng())
	if !ok {
		_ = e.store.RecordSwap(ctx, userID, equipped.ItemHash, equipped.ItemHash, "nothing_to_cycle")
		return ErrNothingToCycle
	}
	if err := e.api.EquipItem(ctx, token, pick.ItemInstanceID, charID, u.MembershipType); err != nil {
		_ = e.store.RecordSwap(ctx, userID, equipped.ItemHash, pick.ItemHash, "error")
		return err
	}
	_ = e.store.RecordSwap(ctx, userID, equipped.ItemHash, pick.ItemHash, "ok")
	log.Printf("cycle: user %d swapped emblem %d -> %d (char %s)", userID, equipped.ItemHash, pick.ItemHash, charID)

	s, err := e.store.GetSettings(ctx, userID)
	if err != nil {
		return err
	}
	s.UserID = userID
	t := now
	s.LastCycledAt = &t
	return e.store.SaveSettings(ctx, s)
}
