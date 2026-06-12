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
