// Coordinator — сервис координации WireGuard mesh-сети.
// Первый узел (Main) запускает coordinator, новые узлы регистрируются через API.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const meshSubnet = "10.100.0.0/16"
const meshIPBase = "10.100.0."

type Node struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	PublicKey string    `json:"public_key"`
	Endpoint  string    `json:"endpoint"`
	VPNIP     string    `json:"vpn_ip"`
	Subdomain string    `json:"subdomain,omitempty"` // e.g. 1-2-3-4 для subdomain.main_domain
	JoinedAt  time.Time `json:"joined_at"`
}

type JoinRequest struct {
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
	Endpoint  string `json:"endpoint"`
	Domain    string `json:"domain"`
}

type JoinResponse struct {
	VPNIP        string   `json:"vpn_ip"`
	Peers        []Peer   `json:"peers"`
	BackupPeers  []string `json:"backup_peers"`
	BackupSecret string   `json:"backup_secret"`
	ListenPort   int      `json:"listen_port"`
}

type Peer struct {
	PublicKey  string `json:"public_key"`
	AllowedIPs string `json:"allowed_ips"`
	Endpoint   string `json:"endpoint,omitempty"`
}

type Coordinator struct {
	mu           sync.Mutex
	nodes        []Node
	nextIP       int
	token        string
	backupSecret string
	storagePath  string
	routesDir    string    // директория для Traefik dynamic routes
	ca           *caLoader // mTLS CA for federation client certs (optional)
}

func (c *Coordinator) load() {
	if c.storagePath == "" {
		return
	}
	data, err := os.ReadFile(c.storagePath)
	if err != nil {
		return
	}
	var state struct {
		Nodes  []Node `json:"nodes"`
		NextIP int    `json:"next_ip"`
		Token  string `json:"token"`
		Backup string `json:"backup_secret"`
	}
	if json.Unmarshal(data, &state) == nil {
		c.nodes = state.Nodes
		c.nextIP = state.NextIP
		if state.Token != "" {
			c.token = state.Token
		}
		if state.Backup != "" {
			c.backupSecret = state.Backup
		}
	}
}

func (c *Coordinator) save() {
	if c.storagePath == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := map[string]interface{}{
		"nodes":         c.nodes,
		"next_ip":       c.nextIP,
		"token":         c.token,
		"backup_secret": c.backupSecret,
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	_ = os.WriteFile(c.storagePath, data, 0600)
}

func (c *Coordinator) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.PublicKey == "" || req.Endpoint == "" || req.Domain == "" {
		http.Error(w, "public_key, endpoint, domain required", http.StatusBadRequest)
		return
	}
	if c.token != "" && req.Token != c.token {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	c.mu.Lock()
	if c.token == "" {
		c.token = randBase64(24)
		c.backupSecret = randBase64(32)
		c.nextIP = 1
	}
	vpnIP := meshIPBase + itoa(c.nextIP)
	c.nextIP++
	node := Node{
		ID:        randBase64(12),
		Domain:    req.Domain,
		PublicKey: req.PublicKey,
		Endpoint:  req.Endpoint,
		VPNIP:     vpnIP,
		JoinedAt:  time.Now(),
	}
	c.nodes = append(c.nodes, node)

	var peers []Peer
	for _, n := range c.nodes {
		if n.VPNIP != vpnIP {
			peers = append(peers, Peer{
				PublicKey:  n.PublicKey,
				AllowedIPs: n.VPNIP + "/32",
				Endpoint:   n.Endpoint,
			})
		}
	}

	var backupPeers []string
	for _, n := range c.nodes {
		if n.VPNIP != vpnIP {
			backupPeers = append(backupPeers, n.VPNIP)
		}
	}
	c.mu.Unlock()
	c.save()

	resp := JoinResponse{
		VPNIP:        vpnIP,
		Peers:        peers,
		BackupPeers:  backupPeers,
		BackupSecret: c.backupSecret,
		ListenPort:   51820,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (c *Coordinator) handlePeers(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if c.token != "" && token != c.token {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	c.mu.Lock()
	nodes := make([]Node, len(c.nodes))
	copy(nodes, c.nodes)
	c.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"nodes": nodes})
}

func subdomainFromEndpoint(endpoint string) string {
	// 1.2.3.4:51820 -> 1-2-3-4
	if idx := strings.Index(endpoint, ":"); idx > 0 {
		endpoint = endpoint[:idx]
	}
	return strings.ReplaceAll(endpoint, ".", "-")
}

func (c *Coordinator) writeTraefikRoute(subdomain, mainDomain, backendURL string) {
	if c.routesDir == "" || subdomain == "" || mainDomain == "" {
		return
	}
	host := subdomain + "." + mainDomain
	safe := strings.NewReplacer(".", "_", "-", "_").Replace(subdomain)
	fpath := c.routesDir + "/selfhost-" + safe + ".yaml"
	body := `http:
  routers:
    ` + safe + `-api:
      rule: "Host(\"` + host + `\") && (PathPrefix(\"/api\") || PathPrefix(\"/.well-known\") || PathPrefix(\"/federation\") || PathPrefix(\"/health\"))"
      service: ` + safe + `-svc
      entryPoints: [websecure]
      tls: { certResolver: letsencrypt }
      priority: 100
    ` + safe + `-web:
      rule: "Host(\"` + host + `\")"
      service: ` + safe + `-svc
      entryPoints: [websecure]
      tls: { certResolver: letsencrypt }
      priority: 1
  services:
    ` + safe + `-svc:
      loadBalancer:
        servers:
          - url: "` + backendURL + `"
        passHostHeader: true
`
	if err := os.WriteFile(fpath, []byte(body), 0644); err != nil {
		log.Printf("write route %s: %v", fpath, err)
	}
}

func (c *Coordinator) handleConfig(w http.ResponseWriter, r *http.Request) {
	// GET /v1/config?token=xxx&public_key=xxx&endpoint=xxx&domain=xxx&use_subdomain=1&main_domain=astralrelay.online
	token := r.URL.Query().Get("token")
	pubkey := r.URL.Query().Get("public_key")
	endpoint := r.URL.Query().Get("endpoint")
	domain := r.URL.Query().Get("domain")
	useSubdomain := r.URL.Query().Get("use_subdomain") == "1" || r.URL.Query().Get("use_subdomain") == "true"
	mainDomain := r.URL.Query().Get("main_domain")
	if pubkey == "" || endpoint == "" || domain == "" {
		http.Error(w, "public_key, endpoint, domain required", http.StatusBadRequest)
		return
	}
	if c.token != "" && token != c.token {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	c.mu.Lock()
	// Повторная регистрация с тем же pubkey — возвращаем существующий конфиг
	for _, n := range c.nodes {
		if n.PublicKey == pubkey {
			vpnIP := n.VPNIP
			secret := c.backupSecret
			subdomain := n.Subdomain
			if subdomain == "" && mainDomain != "" {
				subdomain = subdomainFromEndpoint(n.Endpoint)
				n.Subdomain = subdomain
			}
			if subdomain != "" && mainDomain != "" {
				c.writeTraefikRoute(subdomain, mainDomain, "http://"+vpnIP+":9080")
			}
			var sb strings.Builder
			sb.WriteString("[Interface]\n")
			sb.WriteString("PrivateKey = <PRIVATE_KEY>\n")
			sb.WriteString("Address = " + vpnIP + "/32\n")
			sb.WriteString("ListenPort = 51820\n\n")
			for _, p := range c.nodes {
				if p.VPNIP != vpnIP {
					sb.WriteString("[Peer]\n")
					sb.WriteString("PublicKey = " + p.PublicKey + "\n")
					sb.WriteString("AllowedIPs = " + p.VPNIP + "/32\n")
					sb.WriteString("Endpoint = " + p.Endpoint + "\n\n")
				}
			}
			bp := ""
			for _, p := range c.nodes {
				if p.VPNIP != vpnIP {
					if bp != "" {
						bp += ","
					}
					bp += p.VPNIP
				}
			}
			c.mu.Unlock()
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("X-VPN-IP", vpnIP)
			w.Header().Set("X-Backup-Peers", bp)
			w.Header().Set("X-Backup-Secret", secret)
			if subdomain != "" && mainDomain != "" {
				w.Header().Set("X-Subdomain", subdomain)
				w.Header().Set("X-Server-Domain", subdomain+"."+mainDomain)
			}
			w.Write([]byte(sb.String()))
			return
		}
	}
	if c.token == "" {
		c.token = randBase64(24)
		c.backupSecret = randBase64(32)
		c.nextIP = 1
	}
	vpnIP := meshIPBase + itoa(c.nextIP)
	c.nextIP++
	subdomain := ""
	if useSubdomain && mainDomain != "" {
		subdomain = subdomainFromEndpoint(endpoint)
	}
	node := Node{
		ID:        randBase64(12),
		Domain:    domain,
		PublicKey: pubkey,
		Endpoint:  endpoint,
		VPNIP:     vpnIP,
		Subdomain: subdomain,
		JoinedAt:  time.Now(),
	}
	c.nodes = append(c.nodes, node)
	if subdomain != "" && mainDomain != "" {
		c.writeTraefikRoute(subdomain, mainDomain, "http://"+vpnIP+":9080")
	}

	var sb strings.Builder
	sb.WriteString("[Interface]\n")
	sb.WriteString("PrivateKey = <PRIVATE_KEY>\n")
	sb.WriteString("Address = " + vpnIP + "/32\n")
	sb.WriteString("ListenPort = 51820\n\n")

	for _, n := range c.nodes {
		if n.VPNIP != vpnIP {
			sb.WriteString("[Peer]\n")
			sb.WriteString("PublicKey = " + n.PublicKey + "\n")
			sb.WriteString("AllowedIPs = " + n.VPNIP + "/32\n")
			sb.WriteString("Endpoint = " + n.Endpoint + "\n\n")
		}
	}

	backupPeers := ""
	secret := c.backupSecret
	for _, n := range c.nodes {
		if n.VPNIP != vpnIP {
			if backupPeers != "" {
				backupPeers += ","
			}
			backupPeers += n.VPNIP
		}
	}
	c.mu.Unlock()
	c.save()

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("X-VPN-IP", vpnIP)
	w.Header().Set("X-Backup-Peers", backupPeers)
	w.Header().Set("X-Backup-Secret", secret)
	if subdomain != "" && mainDomain != "" {
		w.Header().Set("X-Subdomain", subdomain)
		w.Header().Set("X-Server-Domain", subdomain+"."+mainDomain)
	}
	w.Write([]byte(sb.String()))
}

// handleCert issues mTLS client certs for federation (POST /v1/cert).
// Requires token and domain; domain must be registered in mesh.
func (c *Coordinator) handleCert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c.ca == nil {
		http.Error(w, "mTLS cert issuance disabled (no COORDINATOR_CA_CERT/KEY)", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Token  string `json:"token"`
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	if domain == "" {
		http.Error(w, "domain required", http.StatusBadRequest)
		return
	}
	if c.token != "" && req.Token != c.token {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	c.mu.Lock()
	var found bool
	for _, n := range c.nodes {
		if strings.EqualFold(n.Domain, domain) {
			found = true
			break
		}
	}
	c.mu.Unlock()
	if !found {
		http.Error(w, "domain not registered in mesh", http.StatusForbidden)
		return
	}
	certPEM, keyPEM, err := c.ca.issueClientCert(domain)
	if err != nil {
		log.Printf("issue cert for %s: %v", domain, err)
		http.Error(w, "failed to issue cert", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"cert_pem": string(certPEM),
		"key_pem":  string(keyPEM),
	})
}

func (c *Coordinator) handleToken(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	tok := c.token
	if tok == "" {
		c.token = randBase64(24)
		c.backupSecret = randBase64(32)
		tok = c.token
	}
	c.mu.Unlock()
	c.save()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": tok})
}

func randBase64(n int) string {
	b := make([]byte, (n*3+3)/4)
	if _, err := rand.Read(b); err != nil {
		b[0] = byte(time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

func itoa(n int) string { return strconv.Itoa(n) }

func main() {
	addr := os.Getenv("COORDINATOR_ADDR")
	if addr == "" {
		addr = ":9443"
	}
	storage := os.Getenv("COORDINATOR_STORAGE")
	if storage == "" {
		storage = "/data/coordinator.json"
	}
	routesDir := os.Getenv("COORDINATOR_ROUTES_DIR")
	caCertPath := os.Getenv("COORDINATOR_CA_CERT")
	caKeyPath := os.Getenv("COORDINATOR_CA_KEY")

	coord := &Coordinator{storagePath: storage, routesDir: routesDir}
	coord.load()
	if caCertPath != "" && caKeyPath != "" {
		if ca, err := loadCA(caCertPath, caKeyPath); err != nil {
			log.Printf("mTLS CA load failed (cert issuance disabled): %v", err)
		} else {
			coord.ca = ca
			log.Printf("mTLS cert issuance enabled")
		}
	}

	http.HandleFunc("/v1/join", coord.handleJoin)
	http.HandleFunc("/v1/config", coord.handleConfig)
	http.HandleFunc("/v1/cert", coord.handleCert)
	http.HandleFunc("/v1/peers", coord.handlePeers)
	http.HandleFunc("/v1/token", coord.handleToken)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })

	log.Printf("Coordinator listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
