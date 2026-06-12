package config

import "testing"

func TestLoad_RequiresEncryptionKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("BUNGIE_API_KEY", "key")
	t.Setenv("BUNGIE_CLIENT_ID", "cid")
	t.Setenv("BUNGIE_CLIENT_SECRET", "secret")
	t.Setenv("BASE_URL", "http://localhost:8080")
	t.Setenv("TOKEN_ENCRYPTION_KEY", "") // missing

	if _, err := Load(); err == nil {
		t.Fatal("expected error when TOKEN_ENCRYPTION_KEY is empty")
	}
}

func TestLoad_RejectsWrongLengthKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("BUNGIE_API_KEY", "key")
	t.Setenv("BUNGIE_CLIENT_ID", "cid")
	t.Setenv("BUNGIE_CLIENT_SECRET", "secret")
	t.Setenv("BASE_URL", "http://localhost:8080")
	t.Setenv("TOKEN_ENCRYPTION_KEY", "tooshort") // present but not 32 bytes

	if _, err := Load(); err == nil {
		t.Fatal("expected error when TOKEN_ENCRYPTION_KEY is not 32 bytes")
	}
}

func TestLoad_Succeeds(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("BUNGIE_API_KEY", "key")
	t.Setenv("BUNGIE_CLIENT_ID", "cid")
	t.Setenv("BUNGIE_CLIENT_SECRET", "secret")
	t.Setenv("BASE_URL", "http://localhost:8080")
	t.Setenv("TOKEN_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef") // 32 bytes

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.BungieAPIKey != "key" || c.BaseURL != "http://localhost:8080" {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestLoad_SecureCookiesFromBaseURL(t *testing.T) {
	base := map[string]bool{
		"https://example.com":   true,
		"http://localhost:8080": false,
	}
	for url, want := range base {
		t.Run(url, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("BUNGIE_API_KEY", "key")
			t.Setenv("BUNGIE_CLIENT_ID", "cid")
			t.Setenv("BUNGIE_CLIENT_SECRET", "secret")
			t.Setenv("BASE_URL", url)
			t.Setenv("TOKEN_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
			c, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.SecureCookies != want {
				t.Fatalf("SecureCookies: got %v, want %v for %s", c.SecureCookies, want, url)
			}
		})
	}
}

func TestLoad_DerivesDistinctHMACKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("BUNGIE_API_KEY", "key")
	t.Setenv("BUNGIE_CLIENT_ID", "cid")
	t.Setenv("BUNGIE_CLIENT_SECRET", "secret")
	t.Setenv("BASE_URL", "http://localhost:8080")
	t.Setenv("TOKEN_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.HMACKey) != 32 {
		t.Fatalf("HMACKey should be 32 bytes, got %d", len(c.HMACKey))
	}
	if string(c.HMACKey) == string(c.EncryptionKey) {
		t.Fatal("HMACKey must be distinct from EncryptionKey")
	}
}
