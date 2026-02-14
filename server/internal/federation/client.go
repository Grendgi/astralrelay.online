package federation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	domain     string
	mainDomain string // if set, send all remote transactions via Main (main_only mode)
	keys       *ServerKeys
	httpClient *http.Client
}

func NewClient(domain string, keys *ServerKeys) *Client {
	return &Client{
		domain:     domain,
		keys:       keys,
		httpClient: httpClientWithTLS(nil, 30*time.Second),
	}
}

// NewClientWithMain creates a client for main_only mode: all remote traffic goes via Main.
func NewClientWithMain(domain, mainDomain string, keys *ServerKeys) *Client {
	c := NewClient(domain, keys)
	c.mainDomain = mainDomain
	return c
}

// NewClientWithMTLS creates a client that uses mTLS (client cert) for federation requests.
func NewClientWithMTLS(domain string, keys *ServerKeys, clientCert, clientKey string) *Client {
	tlsConfig, err := loadClientTLS(clientCert, clientKey)
	if err != nil {
		// Fall back to plain client if cert load fails
		return &Client{
			domain:     domain,
			keys:       keys,
			httpClient: &http.Client{Timeout: 30 * time.Second},
		}
	}
	return &Client{
		domain:     domain,
		keys:       keys,
		httpClient: httpClientWithTLS(tlsConfig, 30*time.Second),
	}
}

// NewClientWithMainAndMTLS combines main_only mode and mTLS.
func NewClientWithMainAndMTLS(domain, mainDomain string, keys *ServerKeys, clientCert, clientKey string) *Client {
	c := NewClientWithMTLS(domain, keys, clientCert, clientKey)
	c.mainDomain = mainDomain
	return c
}

func loadClientTLS(certPath, keyPath string) (*tls.Config, error) {
	if certPath == "" || keyPath == "" {
		return nil, nil
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func httpClientWithTLS(tlsConfig *tls.Config, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

type FederationConfig struct {
	Endpoint string `json:"federation_endpoint"`
	ServerKey string `json:"server_key"`
}

func (c *Client) scheme(domain string) string {
	if strings.HasPrefix(domain, "localhost") || strings.HasPrefix(domain, "127.") {
		return "http"
	}
	return "https"
}

func (c *Client) resolveEndpoint(ctx context.Context, remoteDomain string) (string, error) {
	scheme := c.scheme(remoteDomain)
	url := fmt.Sprintf("%s://%s/.well-known/federation", scheme, remoteDomain)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	base := fmt.Sprintf("%s://%s/federation/v1", c.scheme(remoteDomain), remoteDomain)
	if resp.StatusCode != http.StatusOK {
		return base, nil
	}
	var cfg FederationConfig
	if json.NewDecoder(resp.Body).Decode(&cfg) != nil {
		return base, nil
	}
	if cfg.Endpoint != "" {
		return cfg.Endpoint, nil
	}
	return base, nil
}

func (c *Client) signRequest(method, url, body string) (headers map[string]string) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	dest := ""
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, prefix) {
			if parts := strings.SplitN(url[len(prefix):], "/", 2); len(parts) >= 1 {
				dest = parts[0]
			}
			break
		}
	}
	bodyHash := ""
	if body != "" {
		h := sha256.Sum256([]byte(body))
		bodyHash = hex.EncodeToString(h[:])
	}
	path := "/"
	if idx := strings.Index(url, "://"); idx >= 0 {
		rest := url[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			path = "/" + rest[slash+1:]
		}
	}
	payload := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s", c.domain, dest, timestamp, method, path, bodyHash)
	sig := c.keys.SignPayload([]byte(payload))
	return map[string]string{
		"X-Server-Origin":      c.domain,
		"X-Server-Destination": dest,
		"X-Server-Timestamp":   timestamp,
		"X-Server-Signature":   base64.StdEncoding.EncodeToString(sig),
	}
}

func (c *Client) FetchKeys(ctx context.Context, remoteDomain, userID, deviceID string) ([]byte, error) {
	endpoint, err := c.resolveEndpoint(ctx, remoteDomain)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/keys/bundle/%s/%s", strings.TrimSuffix(endpoint, "/"), userID, deviceID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	for k, v := range c.signRequest("GET", url, "") {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("federation keys: %s", string(data))
	}
	return data, nil
}

type TransactionEvent struct {
	EventID   string `json:"event_id"`
	Type      string `json:"type"`
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	Timestamp int64  `json:"timestamp"`
	Content   struct {
		Ciphertext string `json:"ciphertext"`
		SessionID  string `json:"session_id"`
	} `json:"content"`
}

type Transaction struct {
	TransactionID string              `json:"transaction_id"`
	Origin        string              `json:"origin"`
	Destination   string              `json:"destination"`
	Events        []TransactionEvent  `json:"events"`
}

func (c *Client) SendTransaction(ctx context.Context, remoteDomain string, txn *Transaction) error {
	targetDomain := remoteDomain
	if c.mainDomain != "" && remoteDomain != c.domain {
		targetDomain = c.mainDomain // main_only: route via Main
	}
	txn.Origin = c.domain
	txn.Destination = remoteDomain
	endpoint, err := c.resolveEndpoint(ctx, targetDomain)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(txn)
	url := strings.TrimSuffix(endpoint, "/") + "/transaction"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.signRequest("POST", url, string(body)) {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("federation transaction: %d %s", resp.StatusCode, string(data))
	}
	return nil
}

// ForwardTransaction sends txn to targetDomain preserving Origin/Destination (for Main relay).
func (c *Client) ForwardTransaction(ctx context.Context, targetDomain string, txn *Transaction) error {
	endpoint, err := c.resolveEndpoint(ctx, targetDomain)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(txn)
	url := strings.TrimSuffix(endpoint, "/") + "/transaction"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.signRequest("POST", url, string(body)) {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("federation forward: %d %s", resp.StatusCode, string(data))
	}
	return nil
}
