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
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const discoveryCacheTTL = 5 * time.Minute

type cachedEndpoint struct {
	endpoint string
	expires  time.Time
}

type Client struct {
	domain        string
	mainDomain    string // if set, send all remote transactions via Main (main_only mode)
	keys          *ServerKeys
	httpClient    *http.Client
	discoveryCache map[string]cachedEndpoint
	discoveryMu   sync.RWMutex
}

func NewClient(domain string, keys *ServerKeys) *Client {
	return &Client{
		domain:         domain,
		keys:           keys,
		httpClient:     httpClientWithTLS(nil, 30*time.Second),
		discoveryCache: make(map[string]cachedEndpoint),
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
		return &Client{
			domain:         domain,
			keys:           keys,
			httpClient:     &http.Client{Timeout: 30 * time.Second},
			discoveryCache: make(map[string]cachedEndpoint),
		}
	}
	return &Client{
		domain:         domain,
		keys:           keys,
		httpClient:     httpClientWithTLS(tlsConfig, 30*time.Second),
		discoveryCache: make(map[string]cachedEndpoint),
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
	domain := strings.ToLower(strings.TrimSpace(remoteDomain))
	c.discoveryMu.RLock()
	if ce, ok := c.discoveryCache[domain]; ok && time.Now().Before(ce.expires) {
		c.discoveryMu.RUnlock()
		return ce.endpoint, nil
	}
	c.discoveryMu.RUnlock()

	scheme := c.scheme(remoteDomain)
	url := fmt.Sprintf("%s://%s/.well-known/federation", scheme, remoteDomain)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	base := fmt.Sprintf("%s://%s/federation/v1", c.scheme(remoteDomain), remoteDomain)
	endpoint := base
	if resp.StatusCode == http.StatusOK {
		var cfg FederationConfig
		if json.NewDecoder(resp.Body).Decode(&cfg) == nil && cfg.Endpoint != "" {
			endpoint = cfg.Endpoint
		}
	}

	c.discoveryMu.Lock()
	c.discoveryCache[domain] = cachedEndpoint{endpoint: endpoint, expires: time.Now().Add(discoveryCacheTTL)}
	c.discoveryMu.Unlock()
	return endpoint, nil
}

// FetchServersList fetches list of federation servers from a hub (GET /.well-known/federation-servers).
func (c *Client) FetchServersList(ctx context.Context, hubDomain string) ([]string, error) {
	scheme := c.scheme(hubDomain)
	path := fmt.Sprintf("%s://%s/.well-known/federation-servers", scheme, hubDomain)
	req, _ := http.NewRequestWithContext(ctx, "GET", path, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch servers: %d", resp.StatusCode)
	}
	var out struct {
		Servers []string `json:"servers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	var list []string
	for _, s := range out.Servers {
		d := strings.ToLower(strings.TrimSpace(s))
		if d != "" {
			list = append(list, d)
		}
	}
	return list, nil
}

// Register adds this server as a federation peer on the hub (POST /federation/v1/register).
func (c *Client) Register(ctx context.Context, hubDomain string) error {
	endpoint, err := c.resolveEndpoint(ctx, hubDomain)
	if err != nil {
		return err
	}
	path := strings.TrimSuffix(endpoint, "/") + "/register"
	body := fmt.Sprintf(`{"domain":"%s"}`, strings.ReplaceAll(c.domain, `"`, `\"`))
	req, _ := http.NewRequestWithContext(ctx, "POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.signRequest("POST", path, body) {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register: %d %s", resp.StatusCode, string(data))
	}
	return nil
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

// FetchDeviceList fetches device list for a user from a remote server (for keys lookup).
func (c *Client) FetchDeviceList(ctx context.Context, remoteDomain, userID string) ([]string, error) {
	endpoint, err := c.resolveEndpoint(ctx, remoteDomain)
	if err != nil {
		return nil, err
	}
	path := strings.TrimSuffix(endpoint, "/") + "/keys/devices/" + url.PathEscape(userID)
	req, _ := http.NewRequestWithContext(ctx, "GET", path, nil)
	for k, v := range c.signRequest("GET", path, "") {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("federation devices: %d %s", resp.StatusCode, string(data))
	}
	var out struct {
		Devices []struct {
			DeviceID string `json:"device_id"`
		} `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Devices))
	for _, d := range out.Devices {
		if d.DeviceID != "" {
			ids = append(ids, d.DeviceID)
		}
	}
	return ids, nil
}

func (c *Client) FetchKeys(ctx context.Context, remoteDomain, userID, deviceID string) ([]byte, error) {
	endpoint, err := c.resolveEndpoint(ctx, remoteDomain)
	if err != nil {
		return nil, err
	}
	path := strings.TrimSuffix(endpoint, "/") + "/keys/bundle/" + url.PathEscape(userID) + "/" + url.PathEscape(deviceID)
	req, _ := http.NewRequestWithContext(ctx, "GET", path, nil)
	for k, v := range c.signRequest("GET", path, "") {
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

// UserLookupResult is the response from federation users/lookup (username → home server).
type UserLookupResult struct {
	UserID     string `json:"user_id"`
	HomeDomain string `json:"home_domain"`
}

// UserLookup asks a remote server if it is the home server for the given username. Returns (userID, homeDomain, true) if found, else ("", "", false).
func (c *Client) UserLookup(ctx context.Context, remoteDomain, username string) (userID, homeDomain string, found bool) {
	endpoint, err := c.resolveEndpoint(ctx, remoteDomain)
	if err != nil {
		return "", "", false
	}
	path := strings.TrimSuffix(endpoint, "/") + "/users/lookup"
	urlStr := path + "?username=" + url.QueryEscape(username)
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", "", false
	}
	for k, v := range c.signRequest("GET", urlStr, "") {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", "", false
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	var result UserLookupResult
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return "", "", false
	}
	if result.UserID == "" || result.HomeDomain == "" {
		return "", "", false
	}
	return result.UserID, result.HomeDomain, true
}

// AuthVerifyResult is the response from a federation auth/verify call.
type AuthVerifyResult struct {
	AccessToken string            `json:"access_token"`
	ExpiresIn   int               `json:"expires_in"`
	UserID      string            `json:"user_id"`
	DeviceID    string            `json:"device_id"`
	KeysBackup  map[string]string `json:"keys_backup,omitempty"`
}

// AuthVerify calls the remote server's federation auth/verify to log in a user (for federated login).
func (c *Client) AuthVerify(ctx context.Context, remoteDomain string, body []byte) (*AuthVerifyResult, error) {
	endpoint, err := c.resolveEndpoint(ctx, remoteDomain)
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(endpoint, "/") + "/auth/verify"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.signRequest("POST", url, string(body)) {
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
		return nil, fmt.Errorf("federation auth/verify: %d %s", resp.StatusCode, string(data))
	}
	var result AuthVerifyResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
