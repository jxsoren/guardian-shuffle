package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/cryptobox"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

var key = []byte("0123456789abcdef0123456789abcdef")

func TestValidAccessToken_ReturnsCachedWhenFresh(t *testing.T) {
	box, _ := cryptobox.New(key)
	st := store.NewMemory()
	ctx := context.Background()
	id, _ := st.UpsertUser(ctx, store.User{BungieMembershipID: "m1"})
	accEnc, _ := box.Encrypt([]byte("fresh-access"))
	refEnc, _ := box.Encrypt([]byte("refresh"))
	_ = st.SaveTokens(ctx, store.Tokens{
		UserID: id, AccessTokenEnc: accEnc, RefreshTokenEnc: refEnc,
		AccessExpiresAt: time.Now().Add(30 * time.Minute), RefreshExpiresAt: time.Now().Add(24 * time.Hour),
	})

	tm := NewTokenManager(st, box, "cid", "secret", "https://www.bungie.net", http.DefaultClient)
	tok, err := tm.ValidAccessToken(ctx, id, time.Now())
	if err != nil || tok != "fresh-access" {
		t.Fatalf("got %q err %v", tok, err)
	}
}

func TestValidAccessToken_RefreshesWhenExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"refresh_expires_in":7776000}`))
	}))
	defer srv.Close()

	box, _ := cryptobox.New(key)
	st := store.NewMemory()
	ctx := context.Background()
	id, _ := st.UpsertUser(ctx, store.User{BungieMembershipID: "m1"})
	accEnc, _ := box.Encrypt([]byte("stale"))
	refEnc, _ := box.Encrypt([]byte("old-refresh"))
	_ = st.SaveTokens(ctx, store.Tokens{
		UserID: id, AccessTokenEnc: accEnc, RefreshTokenEnc: refEnc,
		AccessExpiresAt: time.Now().Add(-time.Minute), RefreshExpiresAt: time.Now().Add(24 * time.Hour),
	})

	tm := NewTokenManager(st, box, "cid", "secret", srv.URL, srv.Client())
	tok, err := tm.ValidAccessToken(ctx, id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "new-access" {
		t.Fatalf("expected refreshed token, got %q", tok)
	}
	saved, _ := st.GetTokens(ctx, id)
	dec, _ := box.Decrypt(saved.RefreshTokenEnc)
	if string(dec) != "new-refresh" {
		t.Fatalf("refresh token not rotated, got %q", dec)
	}
}
