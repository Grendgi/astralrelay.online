package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

// SendFederationAlert sends a webhook POST when federation events occur (rate limit, blocklist).
func SendFederationAlert(webhookURL, event, domain string) {
	if webhookURL == "" {
		return
	}
	payload := map[string]string{
		"event":  event,
		"domain": domain,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
