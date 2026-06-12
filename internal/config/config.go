// Package config loads and validates the application's runtime configuration
// from environment variables.
package config

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL        string
	BungieAPIKey       string
	BungieClientID     string
	BungieClientSecret string
	BaseURL            string
	EncryptionKey      []byte // 32 bytes, AES-256
	HMACKey            []byte // 32 bytes, HMAC session signing (HKDF-derived)
	ListenAddr         string
	SecureCookies      bool
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		BungieAPIKey:       os.Getenv("BUNGIE_API_KEY"),
		BungieClientID:     os.Getenv("BUNGIE_CLIENT_ID"),
		BungieClientSecret: os.Getenv("BUNGIE_CLIENT_SECRET"),
		BaseURL:            os.Getenv("BASE_URL"),
		ListenAddr:         envOr("LISTEN_ADDR", ":8080"),
	}
	key := os.Getenv("TOKEN_ENCRYPTION_KEY")
	for name, val := range map[string]string{
		"DATABASE_URL": c.DatabaseURL, "BUNGIE_API_KEY": c.BungieAPIKey,
		"BUNGIE_CLIENT_ID": c.BungieClientID, "BUNGIE_CLIENT_SECRET": c.BungieClientSecret,
		"BASE_URL": c.BaseURL, "TOKEN_ENCRYPTION_KEY": key,
	} {
		if val == "" {
			return Config{}, fmt.Errorf("missing required env %s", name)
		}
	}
	if len(key) != 32 {
		return Config{}, fmt.Errorf("TOKEN_ENCRYPTION_KEY must be 32 bytes, got %d", len(key))
	}
	c.EncryptionKey = []byte(key)
	hmacKey, err := hkdf.Key(sha256.New, c.EncryptionKey, nil, "guardian-shuffle-session-hmac-v1", 32)
	if err != nil {
		return Config{}, fmt.Errorf("hkdf key derivation: %w", err)
	}
	c.HMACKey = hmacKey
	c.SecureCookies = strings.HasPrefix(c.BaseURL, "https://")
	return c, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
