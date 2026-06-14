package swap

import (
	"bytes"
	"context"
	"errors"
	"log"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/bungie"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

type fakeAPI struct {
	profile  *bungie.ProfileResponse
	equipErr error
	equipped []string // itemInstanceIDs passed to EquipItem
}

func (f *fakeAPI) GetProfile(_ context.Context, _ string, _ int64, _ string) (*bungie.ProfileResponse, error) {
	return f.profile, nil
}
func (f *fakeAPI) EquipItem(_ context.Context, _, itemInstanceID, _ string, _ int64) error {
	if f.equipErr != nil {
		return f.equipErr
	}
	f.equipped = append(f.equipped, itemInstanceID)
	return nil
}
func (f *fakeAPI) GetCharacterActivities(_ context.Context, _ string, _ int64, _, _ string) (uint32, error) {
	return 0, nil
}

// staticToken returns a fixed access token; the engine only needs a string.
func staticToken(context.Context, int64, time.Time) (string, error) { return "tok", nil }

func newEngine(api bungie.API, st store.Store) *Engine {
	return NewEngine(api, st, staticToken,
		func() map[uint32]bool { return map[uint32]bool{200: true, 300: true} },
		func() *rand.Rand { return rand.New(rand.NewSource(1)) })
}

func TestCycleUser_LogsTheSwap(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	id, _ := st.UpsertUser(ctx, store.User{BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c1"})

	p := &bungie.ProfileResponse{}
	p.Response.Characters.Data = map[string]bungie.Character{"c1": {CharacterID: "c1", DateLastPlayed: "2026-06-10T00:00:00Z"}}
	p.Response.CharacterEquipment.Data = map[string]bungie.ItemList{
		"c1": {Items: []bungie.Item{{ItemHash: 100, ItemInstanceID: "eqp", BucketHash: bungie.EmblemBucketHash}}},
	}
	p.Response.CharacterInventories.Data = map[string]bungie.ItemList{
		"c1": {Items: []bungie.Item{
			{ItemHash: 200, ItemInstanceID: "inv1", BucketHash: 1469714392},
			{ItemHash: 300, ItemInstanceID: "inv2", BucketHash: 1469714392},
		}},
	}
	api := &fakeAPI{profile: p}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	eng := newEngine(api, st)
	if err := eng.CycleUser(ctx, id, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "cycle:") || !strings.Contains(out, "user 1") {
		t.Fatalf("expected a cycle log line naming user 1, got %q", out)
	}
}

func TestCycleUser_EquipsAnInventoryEmblem(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	id, _ := st.UpsertUser(ctx, store.User{BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c1"})

	p := &bungie.ProfileResponse{}
	p.Response.Characters.Data = map[string]bungie.Character{"c1": {CharacterID: "c1", DateLastPlayed: "2026-06-10T00:00:00Z"}}
	p.Response.CharacterEquipment.Data = map[string]bungie.ItemList{
		"c1": {Items: []bungie.Item{{ItemHash: 100, ItemInstanceID: "eqp", BucketHash: bungie.EmblemBucketHash}}},
	}
	p.Response.CharacterInventories.Data = map[string]bungie.ItemList{
		"c1": {Items: []bungie.Item{
			{ItemHash: 200, ItemInstanceID: "inv1", BucketHash: 1469714392},
			{ItemHash: 300, ItemInstanceID: "inv2", BucketHash: 1469714392},
		}},
	}
	api := &fakeAPI{profile: p}

	eng := newEngine(api, st)
	if err := eng.CycleUser(ctx, id, time.Now()); err != nil {
		t.Fatalf("expected successful swap, got %v", err)
	}
	if len(api.equipped) != 1 {
		t.Fatalf("expected exactly one equip, got %v", api.equipped)
	}
	if api.equipped[0] != "inv1" && api.equipped[0] != "inv2" {
		t.Fatalf("equipped unexpected instance %q", api.equipped[0])
	}
	s, err := st.GetSettings(ctx, id)
	if err != nil || s.LastCycledAt == nil {
		t.Fatalf("expected LastCycledAt to be set after successful cycle, got settings=%+v err=%v", s, err)
	}
}

func TestCycleUser_EquipItemError(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	id, _ := st.UpsertUser(ctx, store.User{BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c1"})
	p := &bungie.ProfileResponse{}
	p.Response.Characters.Data = map[string]bungie.Character{"c1": {CharacterID: "c1", DateLastPlayed: "2026-06-10T00:00:00Z"}}
	p.Response.CharacterEquipment.Data = map[string]bungie.ItemList{
		"c1": {Items: []bungie.Item{{ItemHash: 100, ItemInstanceID: "eqp", BucketHash: bungie.EmblemBucketHash}}},
	}
	p.Response.CharacterInventories.Data = map[string]bungie.ItemList{
		"c1": {Items: []bungie.Item{{ItemHash: 200, ItemInstanceID: "inv1", BucketHash: 1469714392}}},
	}
	api := &fakeAPI{profile: p, equipErr: errors.New("equip failed")}

	eng := newEngine(api, st)
	err := eng.CycleUser(ctx, id, time.Now())
	if err == nil {
		t.Fatal("expected error when EquipItem fails")
	}
	// On equip failure, LastCycledAt must NOT be set.
	s, _ := st.GetSettings(ctx, id)
	if s.LastCycledAt != nil {
		t.Fatalf("LastCycledAt should not be set after a failed equip, got %v", s.LastCycledAt)
	}
}

func TestCycleUser_NoOtherEmblem(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	id, _ := st.UpsertUser(ctx, store.User{BungieMembershipID: "m1", MembershipType: 3, PrimaryCharacterID: "c1"})
	p := &bungie.ProfileResponse{}
	p.Response.Characters.Data = map[string]bungie.Character{"c1": {CharacterID: "c1", DateLastPlayed: "2026-06-10T00:00:00Z"}}
	p.Response.CharacterEquipment.Data = map[string]bungie.ItemList{
		"c1": {Items: []bungie.Item{{ItemHash: 100, ItemInstanceID: "eqp", BucketHash: bungie.EmblemBucketHash}}},
	}
	// inventory has no other emblems
	api := &fakeAPI{profile: p}
	eng := newEngine(api, st)
	if err := eng.CycleUser(ctx, id, time.Now()); err != ErrNothingToCycle {
		t.Fatalf("expected ErrNothingToCycle, got %v", err)
	}
	if len(api.equipped) != 0 {
		t.Fatal("should not have equipped anything")
	}
}
