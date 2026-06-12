package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func (s stubSessions) UserID(*http.Request) (int64, bool)                              { return s.id, s.ok }
func (s stubSessions) SetUserID(http.ResponseWriter, int64)                             {}
func (s stubSessions) SetState(http.ResponseWriter) string                              { return "stub-state" }
func (s stubSessions) ConsumeState(http.ResponseWriter, *http.Request, string) bool     { return true }

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

type errTokens struct{}

func (errTokens) Exchange(context.Context, string) (auth.TokenResponse, error) {
	return auth.TokenResponse{}, fmt.Errorf("dial timeout")
}
func (errTokens) Persist(context.Context, int64, auth.TokenResponse, time.Time) error {
	return nil
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
		Sessions:     stubSessions{},
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

func TestCallback_GenericErrorOnTokenFailure(t *testing.T) {
	h := &Handlers{
		Store:        store.NewMemory(),
		Tokens:       errTokens{},
		Memberships:  stubResolver{},
		Sessions:     stubSessions{},
		AuthorizeURL: "https://www.bungie.net/en/OAuth/Authorize",
	}
	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc", nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "dial timeout") {
		t.Fatalf("body must not expose internal error: %q", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "internal error") {
		t.Fatalf("body should say 'internal error', got %q", w.Body.String())
	}
}

func TestLogin_IncludesStateParam(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), false)
	h := &Handlers{
		ClientID:     "cid",
		AuthorizeURL: "https://www.bungie.net/en/OAuth/Authorize",
		Sessions:     sm,
	}
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	h.Login(w, req)

	loc := w.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("bad redirect URL: %v", err)
	}
	if u.Query().Get("state") == "" {
		t.Fatalf("redirect URL missing state param: %s", loc)
	}
	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "gs_oauth_state" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected gs_oauth_state cookie to be set")
	}
}

func TestCallback_RejectsInvalidState(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), false)
	h := &Handlers{
		Store:        store.NewMemory(),
		Tokens:       &stubTokens{resp: auth.TokenResponse{AccessToken: "acc"}},
		Memberships:  stubResolver{mType: 3, mID: "destiny-id"},
		Sessions:     sm,
		AuthorizeURL: "https://www.bungie.net/en/OAuth/Authorize",
		ClientID:     "cid",
	}
	// Get a real state cookie via Login.
	lw := httptest.NewRecorder()
	h.Login(lw, httptest.NewRequest(http.MethodGet, "/login", nil))

	// Use the wrong state value in the callback.
	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=wrongstate", nil)
	for _, c := range lw.Result().Cookies() {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.Callback(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 on state mismatch, got %d", w.Code)
	}
}

func TestSaveSettings_AcceptsEventMode(t *testing.T) {
	st := store.NewMemory()
	id, _ := st.UpsertUser(context.Background(), store.User{BungieMembershipID: "m1"})

	h := &Handlers{Store: st, Sessions: stubSessions{id: id, ok: true}}
	body := strings.NewReader("mode=event&enabled=on")
	req := httptest.NewRequest(http.MethodPost, "/settings", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.SaveSettings(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", w.Code, w.Body.String())
	}
	got, _ := st.GetSettings(context.Background(), id)
	if got.TriggerMode != "event" {
		t.Fatalf("expected trigger_mode=event, got %q", got.TriggerMode)
	}
}

func TestSaveSettings_UnknownModeFallsBackToManual(t *testing.T) {
	st := store.NewMemory()
	id, _ := st.UpsertUser(context.Background(), store.User{BungieMembershipID: "m2"})

	h := &Handlers{Store: st, Sessions: stubSessions{id: id, ok: true}}
	body := strings.NewReader("mode=bogus&enabled=on")
	req := httptest.NewRequest(http.MethodPost, "/settings", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.SaveSettings(w, req)

	got, _ := st.GetSettings(context.Background(), id)
	if got.TriggerMode != "manual" {
		t.Fatalf("expected fallback to manual, got %q", got.TriggerMode)
	}
}

func TestCallback_AcceptsMatchingState(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), false)
	h := &Handlers{
		Store:        store.NewMemory(),
		Tokens:       &stubTokens{resp: auth.TokenResponse{AccessToken: "acc"}},
		Memberships:  stubResolver{mType: 3, mID: "destiny-id"},
		Sessions:     sm,
		AuthorizeURL: "https://www.bungie.net/en/OAuth/Authorize",
		ClientID:     "cid",
	}
	// Get the state nonce from Login.
	lw := httptest.NewRecorder()
	h.Login(lw, httptest.NewRequest(http.MethodGet, "/login", nil))
	loc := lw.Header().Get("Location")
	u, _ := url.Parse(loc)
	nonce := u.Query().Get("state")

	// Callback with the matching nonce.
	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state="+nonce, nil)
	for _, c := range lw.Result().Cookies() {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.Callback(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", w.Code, w.Body.String())
	}
}
