package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
)

const sessionCookie = "gs_session"

// CookieSessions stores the user ID in a cookie with an HMAC signature.
type CookieSessions struct {
	key    []byte
	secure bool
}

func NewCookieSessions(key []byte, secure bool) *CookieSessions {
	return &CookieSessions{key: key, secure: secure}
}

var _ SessionManager = (*CookieSessions)(nil)

func (s *CookieSessions) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *CookieSessions) SetUserID(w http.ResponseWriter, userID int64) {
	payload := strconv.FormatInt(userID, 10)
	value := s.sign(payload) + "|" + payload
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secure,
	})
}

func (s *CookieSessions) UserID(r *http.Request) (int64, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return 0, false
	}
	parts := strings.SplitN(c.Value, "|", 2)
	if len(parts) != 2 {
		return 0, false
	}
	if !hmac.Equal([]byte(parts[0]), []byte(s.sign(parts[1]))) {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
