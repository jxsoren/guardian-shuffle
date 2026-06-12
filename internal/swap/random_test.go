package swap

import (
	"math/rand"
	"testing"

	"github.com/jsorensen/guardian_shuffle/internal/bungie"
)

func TestPickRandom_Empty(t *testing.T) {
	_, ok := PickRandom(nil, rand.New(rand.NewSource(1)))
	if ok {
		t.Fatal("expected ok=false for empty pool")
	}
}

func TestPickRandom_Single(t *testing.T) {
	pool := []bungie.Item{{ItemInstanceID: "only"}}
	got, ok := PickRandom(pool, rand.New(rand.NewSource(1)))
	if !ok || got.ItemInstanceID != "only" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

func TestPickRandom_WithinPool(t *testing.T) {
	pool := []bungie.Item{{ItemInstanceID: "a"}, {ItemInstanceID: "b"}, {ItemInstanceID: "c"}}
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 50; i++ {
		got, ok := PickRandom(pool, r)
		if !ok {
			t.Fatal("expected ok")
		}
		if got.ItemInstanceID != "a" && got.ItemInstanceID != "b" && got.ItemInstanceID != "c" {
			t.Fatalf("picked item outside pool: %+v", got)
		}
	}
}
