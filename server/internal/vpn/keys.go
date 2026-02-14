package vpn

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/google/uuid"
	"golang.org/x/crypto/curve25519"
)

// GenerateXrayUUID returns a new UUID for VMess/VLESS (format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx).
func GenerateXrayUUID() string {
	return uuid.New().String()
}

// GenerateTrojanPassword returns a random 24-byte base64 password for Trojan (32 chars).
func GenerateTrojanPassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:32], nil
}

// WireGuardKeyPair generates a WireGuard-compatible key pair.
func WireGuardKeyPair() (privKey, pubKey string, err error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", err
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)

	return base64.StdEncoding.EncodeToString(priv[:]), base64.StdEncoding.EncodeToString(pub[:]), nil
}
