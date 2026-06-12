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
