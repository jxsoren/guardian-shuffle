package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/auth"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

type stubCycler struct{ err error }

func (s stubCycler) CycleUser(context.Context, int64, time.Time) error { return s.err }

type stubSessions struct {
	id int64
	ok bool
}

func (s stubSessions) UserID(*http.Request) (int64, bool)   { return s.id, s.ok }
func (s stubSessions) SetUserID(http.ResponseWriter, int64) {}

type stubTokens struct {
	resp      auth.TokenResponse
	persisted bool
}

func (s *stubTokens) Exchange(context.Context, string) (auth.TokenResponse, error) {
	return s.resp, nil
}
func (s *stubTokens) Persist(_ context.Context, _ int64, _ auth.TokenResponse, _ time.Time) error {
	s.persisted = true
	return nil
}

type stubResolver struct {
	mType int64
	mID   string
}

func (s stubResolver) PrimaryDestinyMembership(context.Context, string) (int64, string, error) {
	return s.mType, s.mID, nil
}

func TestCallback_ResolvesAndStoresDestinyMembership(t *testing.T) {
	st := store.NewMemory()
	tk := &stubTokens{resp: auth.TokenResponse{AccessToken: "acc", MembershipID: "bungienet-id"}}
	h := &Handlers{
		Store:        st,
		Tokens:       tk,
		Memberships:  stubResolver{mType: 3, mID: "destiny-id"},
		Sessions:     stubSessions{id: 0, ok: true},
		AuthorizeURL: "https://www.bungie.net/en/OAuth/Authorize",
	}
	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc", nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("want 302 redirect, got %d (body %q)", w.Code, w.Body.String())
	}
	if !tk.persisted {
		t.Fatal("expected tokens to be persisted")
	}
	// The stored user must carry the DESTINY membership type + id (not the BungieNet id).
	u, err := st.GetUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("user not stored: %v", err)
	}
	if u.MembershipType != 3 || u.BungieMembershipID != "destiny-id" {
		t.Fatalf("stored wrong membership: type=%d id=%q", u.MembershipType, u.BungieMembershipID)
	}
}

func TestLoginRedirectsToBungie(t *testing.T) {
	h := &Handlers{
		ClientID:     "cid",
		BaseURL:      "http://localhost:8080",
		AuthorizeURL: "https://www.bungie.net/en/OAuth/Authorize",
	}
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "client_id=cid") || !strings.Contains(loc, "response_type=code") {
		t.Fatalf("bad authorize redirect: %s", loc)
	}
}

func TestCycleNow_RequiresSession(t *testing.T) {
	h := &Handlers{Cycler: stubCycler{}, Sessions: stubSessions{ok: false}}
	req := httptest.NewRequest(http.MethodPost, "/cycle-now", nil)
	w := httptest.NewRecorder()
	h.CycleNow(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without session, got %d", w.Code)
	}
}

func TestCycleNow_WithSession(t *testing.T) {
	h := &Handlers{Cycler: stubCycler{}, Sessions: stubSessions{id: 1, ok: true}}
	req := httptest.NewRequest(http.MethodPost, "/cycle-now", nil)
	w := httptest.NewRecorder()
	h.CycleNow(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 with session, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "cycled") {
		t.Fatalf("expected success message, got %q", w.Body.String())
	}
}
