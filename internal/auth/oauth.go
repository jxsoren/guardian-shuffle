// Package auth handles Bungie OAuth token exchange, refresh, and encrypted persistence.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/cryptobox"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

const refreshSkew = 60 * time.Second // refresh a bit early to avoid races

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	MembershipID     string `json:"membership_id"`
}

// ErrReauthRequired signals the refresh token is dead and the user must sign in again.
var ErrReauthRequired = fmt.Errorf("re-authentication required")

type TokenManager struct {
	store        store.Store
	box          *cryptobox.Box
	clientID     string
	clientSecret string
	tokenBase    string // base URL hosting /Platform/App/OAuth/Token/
	http         *http.Client
}

func NewTokenManager(s store.Store, box *cryptobox.Box, clientID, clientSecret, tokenBase string, hc *http.Client) *TokenManager {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &TokenManager{store: s, box: box, clientID: clientID, clientSecret: clientSecret, tokenBase: tokenBase, http: hc}
}

// ValidAccessToken returns a usable access token, refreshing if the stored one is expired.
func (tm *TokenManager) ValidAccessToken(ctx context.Context, userID int64, now time.Time) (string, error) {
	tk, err := tm.store.GetTokens(ctx, userID)
	if err != nil {
		return "", err
	}
	if now.Add(refreshSkew).Before(tk.AccessExpiresAt) {
		acc, err := tm.box.Decrypt(tk.AccessTokenEnc)
		if err != nil {
			return "", err
		}
		return string(acc), nil
	}
	if !now.Before(tk.RefreshExpiresAt) {
		return "", ErrReauthRequired
	}
	refresh, err := tm.box.Decrypt(tk.RefreshTokenEnc)
	if err != nil {
		return "", err
	}
	resp, err := tm.requestToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {string(refresh)},
	})
	if err != nil {
		return "", err
	}
	if err := tm.persist(ctx, userID, resp, now); err != nil {
		return "", err
	}
	return resp.AccessToken, nil
}

// Exchange swaps an authorization code for tokens (used by the OAuth callback).
func (tm *TokenManager) Exchange(ctx context.Context, code string) (tokenResponse, error) {
	return tm.requestTokenValue(ctx, url.Values{
		"grant_type": {"authorization_code"},
		"code":       {code},
	})
}

func (tm *TokenManager) requestToken(ctx context.Context, form url.Values) (tokenResponse, error) {
	return tm.requestTokenValue(ctx, form)
}

func (tm *TokenManager) requestTokenValue(ctx context.Context, form url.Values) (tokenResponse, error) {
	form.Set("client_id", tm.clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		tm.tokenBase+"/Platform/App/OAuth/Token/", strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(tm.clientID, tm.clientSecret)

	resp, err := tm.http.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return tokenResponse{}, fmt.Errorf("token endpoint status %d", resp.StatusCode)
	}
	var out tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return tokenResponse{}, err
	}
	return out, nil
}

// Persist encrypts and stores a token response. Exported for the OAuth callback.
func (tm *TokenManager) Persist(ctx context.Context, userID int64, resp tokenResponse, now time.Time) error {
	return tm.persist(ctx, userID, resp, now)
}

func (tm *TokenManager) persist(ctx context.Context, userID int64, resp tokenResponse, now time.Time) error {
	accEnc, err := tm.box.Encrypt([]byte(resp.AccessToken))
	if err != nil {
		return err
	}
	refEnc, err := tm.box.Encrypt([]byte(resp.RefreshToken))
	if err != nil {
		return err
	}
	return tm.store.SaveTokens(ctx, store.Tokens{
		UserID:           userID,
		AccessTokenEnc:   accEnc,
		RefreshTokenEnc:  refEnc,
		AccessExpiresAt:  now.Add(time.Duration(resp.ExpiresIn) * time.Second),
		RefreshExpiresAt: now.Add(time.Duration(resp.RefreshExpiresIn) * time.Second),
	})
}
