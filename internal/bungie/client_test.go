package bungie

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetProfile_ParsesAndSendsAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("missing API key header")
		}
		if !strings.Contains(r.URL.String(), "components=100,102,200,201,205") {
			t.Errorf("missing/incorrect components in URL: %s", r.URL.String())
		}
		if !strings.Contains(r.URL.Path, "/Platform/Destiny2/3/Profile/m1/") {
			t.Errorf("unexpected profile path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Response":{"characters":{"data":{"c1":{"characterId":"c1","dateLastPlayed":"2026-06-10T00:00:00Z"}}}},"ErrorCode":1}`))
	}))
	defer srv.Close()

	c := NewClient("test-key", srv.URL, srv.Client())
	p, err := c.GetProfile(context.Background(), "tok", 3, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Response.Characters.Data["c1"].CharacterID != "c1" {
		t.Fatalf("parse failed: %+v", p)
	}
}

func TestEquipItem_NonOneErrorCodeIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %q", r.Header.Get("Content-Type"))
		}
		b, _ := io.ReadAll(r.Body)
		body := string(b)
		if !strings.Contains(body, `"itemId":"inst"`) || !strings.Contains(body, `"characterId":"char"`) || !strings.Contains(body, `"membershipType":3`) {
			t.Errorf("unexpected equip body: %s", body)
		}
		_, _ = w.Write([]byte(`{"ErrorCode":1665,"ErrorStatus":"DestinyItemActionForbidden","Message":"in activity"}`))
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, srv.Client())
	err := c.EquipItem(context.Background(), "tok", "inst", "char", 3)
	if err == nil {
		t.Fatal("expected error for non-1 ErrorCode")
	}
}

func TestGetEmblemHashSet_FiltersByCategory(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/Platform/Destiny2/Manifest/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Response":{"jsonWorldComponentContentPaths":{"en":{"DestinyInventoryItemDefinition":"/defs.json"}}},"ErrorCode":1}`))
	})
	mux.HandleFunc("/defs.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"100":{"itemCategoryHashes":[19]},"200":{"itemCategoryHashes":[1]},"300":{"itemCategoryHashes":[42,19]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient("k", srv.URL, srv.Client())
	set, err := c.GetEmblemHashSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !set[100] || !set[300] || set[200] {
		t.Fatalf("unexpected emblem set: %+v", set)
	}
}
