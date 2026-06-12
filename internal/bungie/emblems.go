package bungie

import "fmt"

// ActiveCharacterID returns the most-recently-played character's ID.
func ActiveCharacterID(p *ProfileResponse) (string, error) {
	best, bestDate := "", ""
	for id, c := range p.Response.Characters.Data {
		if c.DateLastPlayed > bestDate { // RFC3339 sorts lexicographically
			best, bestDate = id, c.DateLastPlayed
		}
	}
	if best == "" {
		return "", fmt.Errorf("no characters found")
	}
	return best, nil
}

// EquippedEmblem returns the emblem currently equipped on the character.
func EquippedEmblem(p *ProfileResponse, charID string) (Item, bool) {
	for _, it := range p.Response.CharacterEquipment.Data[charID].Items {
		if it.BucketHash == EmblemBucketHash {
			return it, true
		}
	}
	return Item{}, false
}

// EmblemPool returns owned emblem instances for the character, excluding the
// currently equipped one. Emblems are identified by membership in emblemHashes
// (sourced from the manifest), since stored emblems live in generic buckets.
func EmblemPool(p *ProfileResponse, charID string, emblemHashes map[uint32]bool) []Item {
	equipped, _ := EquippedEmblem(p, charID)
	var pool []Item
	add := func(items []Item) {
		for _, it := range items {
			if it.ItemInstanceID == "" || it.ItemInstanceID == equipped.ItemInstanceID {
				continue
			}
			if emblemHashes[it.ItemHash] {
				pool = append(pool, it)
			}
		}
	}
	add(p.Response.CharacterInventories.Data[charID].Items)
	add(p.Response.ProfileInventory.Data.Items)
	return pool
}
