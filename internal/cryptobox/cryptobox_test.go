package cryptobox

import "testing"

var key = []byte("0123456789abcdef0123456789abcdef") // 32 bytes

func TestRoundTrip(t *testing.T) {
	box, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := box.Encrypt([]byte("secret-token"))
	if err != nil {
		t.Fatal(err)
	}
	if string(enc) == "secret-token" {
		t.Fatal("ciphertext must not equal plaintext")
	}
	dec, err := box.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec) != "secret-token" {
		t.Fatalf("got %q", dec)
	}
}

func TestEncryptIsNonDeterministic(t *testing.T) {
	box, _ := New(key)
	a, _ := box.Encrypt([]byte("x"))
	b, _ := box.Encrypt([]byte("x"))
	if string(a) == string(b) {
		t.Fatal("two encryptions of same plaintext must differ (random nonce)")
	}
}
