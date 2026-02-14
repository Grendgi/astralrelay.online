package federation

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func FetchServerKey(ctx context.Context, domain string) (ed25519.PublicKey, error) {
	scheme := "https"
	if strings.HasPrefix(domain, "localhost") || strings.HasPrefix(domain, "127.") {
		scheme = "http"
	}
	url := fmt.Sprintf("%s://%s/.well-known/federation", scheme, domain)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("federation discovery: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var cfg struct {
		ServerKey string `json:"server_key"`
	}
	if json.Unmarshal(body, &cfg) != nil {
		return nil, fmt.Errorf("invalid federation config")
	}
	key, err := base64.StdEncoding.DecodeString(cfg.ServerKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid server key")
	}
	return ed25519.PublicKey(key), nil
}
