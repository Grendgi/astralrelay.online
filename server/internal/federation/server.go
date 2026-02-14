package federation

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// VerifyRequest verifies S2S signature. originPublicKey is the key of the originating server (from X-Server-Origin).
func VerifyRequest(r *http.Request, body []byte, originPublicKey ed25519.PublicKey) error {
	origin := r.Header.Get("X-Server-Origin")
	dest := r.Header.Get("X-Server-Destination")
	ts := r.Header.Get("X-Server-Timestamp")
	sig := r.Header.Get("X-Server-Signature")
	if origin == "" || dest == "" || ts == "" || sig == "" {
		return fmt.Errorf("missing S2S headers")
	}
	timestamp, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	if time.Now().Unix()-timestamp > 300 || timestamp-time.Now().Unix() > 300 {
		return fmt.Errorf("timestamp out of window")
	}
	bodyHash := ""
	if len(body) > 0 {
		h := sha256.Sum256(body)
		bodyHash = hex.EncodeToString(h[:])
	}
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	payload := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s", origin, dest, ts, r.Method, path, bodyHash)
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return err
	}
	if !ed25519.Verify(originPublicKey, []byte(payload), sigBytes) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type TransactionRequest struct {
	TransactionID string `json:"transaction_id"`
	Origin        string `json:"origin"`
	Destination   string `json:"destination"`
	Events        []struct {
		EventID   string `json:"event_id"`
		Type      string `json:"type"`
		Sender    string `json:"sender"`
		Recipient string `json:"recipient"`
		Timestamp int64  `json:"timestamp"`
		Content   struct {
			Ciphertext string `json:"ciphertext"`
			SessionID  string `json:"session_id"`
		} `json:"content"`
	} `json:"events"`
}

type TransactionResponse struct {
	Accepted []string      `json:"accepted"`
	Rejected []interface{} `json:"rejected"`
}
