package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCookieSession_RoundTrip(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), false)
	w := httptest.NewRecorder()
	sm.SetUserID(w, 42)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}
	id, ok := sm.UserID(req)
	if !ok || id != 42 {
		t.Fatalf("got id=%d ok=%v", id, ok)
	}
}

func TestCookieSession_SecureFlag(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), true)
	w := httptest.NewRecorder()
	sm.SetUserID(w, 1)
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a cookie")
	}
	if !cookies[0].Secure {
		t.Fatal("expected Secure flag to be set")
	}
}

func TestCookieSession_RejectsTampered(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: "deadbeef|42"})
	if _, ok := sm.UserID(req); ok {
		t.Fatal("tampered cookie must be rejected")
	}
}

func TestCookieSession_StateRoundTrip(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), false)

	// SetState returns a nonce and sets a cookie.
	lw := httptest.NewRecorder()
	nonce := sm.SetState(lw)
	if nonce == "" {
		t.Fatal("expected non-empty nonce")
	}

	// ConsumeState with the same nonce succeeds.
	req := httptest.NewRequest(http.MethodGet, "/callback?state="+nonce, nil)
	for _, c := range lw.Result().Cookies() {
		req.AddCookie(c)
	}
	cw := httptest.NewRecorder()
	if !sm.ConsumeState(cw, req, nonce) {
		t.Fatal("expected ConsumeState to return true for matching nonce")
	}
}

func TestCookieSession_StateRejectsMismatch(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), false)

	lw := httptest.NewRecorder()
	sm.SetState(lw)

	req := httptest.NewRequest(http.MethodGet, "/callback?state=wrongnonce", nil)
	for _, c := range lw.Result().Cookies() {
		req.AddCookie(c)
	}
	cw := httptest.NewRecorder()
	if sm.ConsumeState(cw, req, "wrongnonce") {
		t.Fatal("expected ConsumeState to return false for mismatched nonce")
	}
}

func TestCookieSession_StateRejectsNoCookie(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), false)
	req := httptest.NewRequest(http.MethodGet, "/callback?state=anything", nil)
	cw := httptest.NewRecorder()
	if sm.ConsumeState(cw, req, "anything") {
		t.Fatal("expected ConsumeState to return false when no state cookie present")
	}
}

func TestCookieSession_StateRejectsTamperedCookie(t *testing.T) {
	sm := NewCookieSessions([]byte("0123456789abcdef0123456789abcdef"), false)
	req := httptest.NewRequest(http.MethodGet, "/callback?state=anything", nil)
	req.AddCookie(&http.Cookie{Name: "gs_oauth_state", Value: "deadbeef|anything"})
	cw := httptest.NewRecorder()
	if sm.ConsumeState(cw, req, "anything") {
		t.Fatal("tampered state cookie must be rejected")
	}
}
