package dbenc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

const magicPrefix = "DBE1" // database encryption v1

var ErrInvalidKey = errors.New("DB_ENCRYPTION_KEY must be 32 bytes base64")
var ErrDecrypt = errors.New("decryption failed")

// Cipher wraps AES-256-GCM for DB column encryption.
// If key is nil/empty, Encrypt returns plaintext and Decrypt returns as-is (no-op).
type Cipher struct {
	aead cipher.AEAD
}

// New creates a Cipher from base64-encoded 32-byte key.
// Empty key returns nil - caller should treat as no encryption.
func New(keyB64 string) (*Cipher, error) {
	if keyB64 == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt encrypts plaintext. Format: magic(4) + nonce(12) + ciphertext + tag(16).
// If c is nil, returns plaintext unchanged.
func (c *Cipher) Encrypt(plain []byte) ([]byte, error) {
	if c == nil {
		return plain, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := c.aead.Seal(nil, nonce, plain, nil)
	out := make([]byte, 0, len(magicPrefix)+len(nonce)+len(ct))
	out = append(out, magicPrefix...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// Decrypt decrypts data. If not our format (magic prefix), returns as-is (legacy plaintext).
func (c *Cipher) Decrypt(data []byte) ([]byte, error) {
	if c == nil || len(data) < len(magicPrefix) || string(data[:len(magicPrefix)]) != magicPrefix {
		return data, nil
	}
	data = data[len(magicPrefix):]
	nonceSize := c.aead.NonceSize()
	if len(data) < nonceSize {
		return nil, ErrDecrypt
	}
	nonce := data[:nonceSize]
	ct := data[nonceSize:]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}

// Enabled returns true if encryption is active.
func (c *Cipher) Enabled() bool {
	return c != nil
}

// GenerateKey returns a base64-encoded 32-byte key for DB_ENCRYPTION_KEY.
func GenerateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// IsEncrypted returns true if data looks like our encrypted format.
func IsEncrypted(data []byte) bool {
	return len(data) >= len(magicPrefix) && bytes.Equal([]byte(magicPrefix), data[:len(magicPrefix)])
}
