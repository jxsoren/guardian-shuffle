package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type stubCycler struct{ err error }

func (s stubCycler) CycleUser(context.Context, int64, time.Time) error { return s.err }

type stubSessions struct {
	id int64
	ok bool
}

func (s stubSessions) UserID(*http.Request) (int64, bool) { return s.id, s.ok }
func (s stubSessions) SetUserID(http.ResponseWriter, int64) {}

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
