// Package swap implements the emblem-cycling swap logic for Guardian Shuffle.
package swap

import (
	"math/rand"

	"github.com/jsorensen/guardian_shuffle/internal/bungie"
)

// PickRandom returns a random item from the pool. ok is false if the pool is empty.
// The caller is responsible for having already excluded the equipped emblem.
func PickRandom(pool []bungie.Item, r *rand.Rand) (bungie.Item, bool) {
	if len(pool) == 0 {
		return bungie.Item{}, false
	}
	return pool[r.Intn(len(pool))], true
}
