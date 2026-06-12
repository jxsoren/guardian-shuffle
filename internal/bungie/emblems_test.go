package bungie

import "testing"

func sampleProfile() *ProfileResponse {
	p := &ProfileResponse{}
	p.Response.Characters.Data = map[string]Character{
		"charOld": {CharacterID: "charOld", DateLastPlayed: "2026-06-01T00:00:00Z"},
		"charNew": {CharacterID: "charNew", DateLastPlayed: "2026-06-10T00:00:00Z"},
	}
	p.Response.CharacterEquipment.Data = map[string]ItemList{
		"charNew": {Items: []Item{{ItemHash: 100, ItemInstanceID: "eqp", BucketHash: EmblemBucketHash}}},
	}
	p.Response.CharacterInventories.Data = map[string]ItemList{
		"charNew": {Items: []Item{
			{ItemHash: 200, ItemInstanceID: "inv1", BucketHash: 1469714392},
			{ItemHash: 300, ItemInstanceID: "inv2", BucketHash: 1469714392},
			{ItemHash: 999, ItemInstanceID: "weapon", BucketHash: 1498876634}, // not an emblem
		}},
	}
	return p
}

func TestActiveCharacterID(t *testing.T) {
	got, err := ActiveCharacterID(sampleProfile())
	if err != nil {
		t.Fatal(err)
	}
	if got != "charNew" {
		t.Fatalf("got %q, want charNew", got)
	}
}

func TestEquippedEmblem(t *testing.T) {
	got, ok := EquippedEmblem(sampleProfile(), "charNew")
	if !ok || got.ItemInstanceID != "eqp" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

func TestEmblemPool_FiltersByHashSetAndExcludesEquipped(t *testing.T) {
	emblemHashes := map[uint32]bool{100: true, 200: true, 300: true}
	pool := EmblemPool(sampleProfile(), "charNew", emblemHashes)
	// 100 is equipped (excluded), 999 is not an emblem (excluded) -> {200,300}
	if len(pool) != 2 {
		t.Fatalf("want 2 candidates, got %d: %+v", len(pool), pool)
	}
	for _, it := range pool {
		if it.ItemHash == 100 || it.ItemHash == 999 {
			t.Fatalf("unexpected item in pool: %+v", it)
		}
	}
}
