// Package cryptobox provides AES-256-GCM authenticated encryption for secrets such as OAuth tokens.
package cryptobox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

type Box struct{ gcm cipher.AEAD }

func New(key []byte) (*Box, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{gcm: gcm}, nil
}

func (b *Box) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return b.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (b *Box) Decrypt(ciphertext []byte) ([]byte, error) {
	ns := b.gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	return b.gcm.Open(nil, nonce, ct, nil)
}
