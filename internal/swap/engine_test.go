package swap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
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
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	eng := newEngine(api, st)
	if err := eng.CycleUser(ctx, id, time.Now(), "manual"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec := findLogRecord(t, buf.String(), "cycle")
	if rec["trigger"] != "manual" {
		t.Fatalf("expected trigger=manual, got %v", rec["trigger"])
	}
	if rec["result"] != "ok" {
		t.Fatalf("expected result=ok, got %v", rec["result"])
	}
	// JSON numbers decode as float64.
	if rec["user_id"] != float64(id) {
		t.Fatalf("expected user_id=%d, got %v", id, rec["user_id"])
	}
	if _, ok := rec["old_hash"]; !ok {
		t.Fatalf("expected old_hash field, got %v", rec)
	}
	if _, ok := rec["new_hash"]; !ok {
		t.Fatalf("expected new_hash field, got %v", rec)
	}
}

// findLogRecord parses newline-delimited slog JSON and returns the first record
// whose "msg" equals want.
func findLogRecord(t *testing.T, out, want string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q (%v)", line, err)
		}
		if rec["msg"] == want {
			return rec
		}
	}
	t.Fatalf("no log record with msg=%q in:\n%s", want, out)
	return nil
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
	if err := eng.CycleUser(ctx, id, time.Now(), "manual"); err != nil {
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
	err := eng.CycleUser(ctx, id, time.Now(), "manual")
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
	if err := eng.CycleUser(ctx, id, time.Now(), "manual"); err != ErrNothingToCycle {
		t.Fatalf("expected ErrNothingToCycle, got %v", err)
	}
	if len(api.equipped) != 0 {
		t.Fatal("should not have equipped anything")
	}
}
