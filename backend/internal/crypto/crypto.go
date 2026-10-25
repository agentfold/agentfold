// Package crypto seals secrets stored in the database (agent credentials,
// MCP server secrets) with AES-256-GCM.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// KeySize is the length of the key in bytes.
const KeySize = 32

// ErrOpen means a sealed value could not be decrypted: wrong key or tampered data.
var ErrOpen = errors.New("cannot decrypt secret: wrong ORQ_SECRET_KEY or corrupted data")

// Box seals and opens secrets with one key.
type Box struct {
	aead cipher.AEAD
	rand io.Reader
}

// NewKey returns a random key encoded as ORQ_SECRET_KEY expects.
func NewKey() string {
	key := make([]byte, KeySize)
	_, _ = rand.Read(key) // crypto/rand.Read never fails
	return base64.StdEncoding.EncodeToString(key)
}

// NewBox builds a Box from a base64 key of KeySize bytes.
func NewBox(encodedKey string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != KeySize {
		return nil, fmt.Errorf("the secret key must be %d bytes in base64 (generate one with `orquestrator secret-key`)", KeySize)
	}
	block, _ := aes.NewCipher(key)  // only fails for invalid key sizes
	aead, _ := cipher.NewGCM(block) // only fails for unusual block sizes
	return &Box{aead: aead, rand: rand.Reader}, nil
}

// Seal encrypts plaintext; the nonce is prepended to the result.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(b.rand, nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a value produced by Seal.
func (b *Box) Open(sealed []byte) ([]byte, error) {
	size := b.aead.NonceSize()
	if len(sealed) < size {
		return nil, ErrOpen
	}
	plaintext, err := b.aead.Open(nil, sealed[:size], sealed[size:], nil)
	if err != nil {
		return nil, ErrOpen
	}
	return plaintext, nil
}
