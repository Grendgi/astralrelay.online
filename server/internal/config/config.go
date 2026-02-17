package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	S3         S3Config
	Redis      RedisConfig
	Federation FederationConfig
	VPN        VPNConfig
	Push       PushConfig
}

type PushConfig struct {
	VAPIDPublicKey  string
	VAPIDPrivateKey string
}

type VPNConfig struct {
	Enabled           bool
	DefaultExpiryDays int
	NodesJSON         string // optional: JSON array of nodes to seed
	WireGuard         WireGuardVPNConfig
	OpenVPNTCP443     OpenVPNVPNConfig
	Xray              XrayVPNConfig
}

type XrayVPNConfig struct {
	Enabled    bool
	Endpoint   string // host:port for client URLs, e.g. vpn.example.org:443
	APIAddr    string // gRPC API address for AddUser/RemoveUser, e.g. xray:10085 (empty = skip API)
	VmessPort  int    // override port for VMess URL (0 = use from Endpoint)
	VlessPort  int    // override port for VLESS URL
	TrojanPort int    // override port for Trojan URL
}

type WireGuardVPNConfig struct {
	Enabled        bool
	Endpoint       string
	ServerPubKey   string
	ClientSubnet   string
	StatsInterface string // e.g. "wg0" - if set, fetch traffic via `wg show`
}

type OpenVPNVPNConfig struct {
	Enabled  bool
	Endpoint string
}

type ServerConfig struct {
	Domain         string
	Port           int
	Debug          bool
	E2EEStrictOnly bool // if true, reject non-Signal (MVP/plain) messages
}

type DatabaseConfig struct {
	URL           string
	FederationURL string // optional: messenger_federation user for AcceptTransaction
	EncryptionKey string // base64 32-byte, optional; if set, sensitive columns are encrypted
}

type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	Region    string
}

type RedisConfig struct {
	URL      string
	Disabled bool
}

type FederationConfig struct {
	Endpoint   string   // e.g. https://example.org/federation
	Enabled    bool
	Peers      []string // FEDERATION_PEERS: bootstrap domains for user lookup & login (e.g. astralrelay.online,82.97.250.36.nip.io)
	Security   FederationSecurityConfig
	Mode       string // open, main_only
	MainDomain string // for main_only: only accept from & send via this domain
	MTLS       FederationMTLSConfig
}

type FederationMTLSConfig struct {
	ClientCert string // path to client cert (PEM)
	ClientKey  string // path to client key (PEM)
}

type FederationSecurityConfig struct {
	RateLimit               int    // per domain per minute
	MaxBodySize             int    // bytes
	AllowlistMode           string // auto, manual, open
	AllowlistPath           string
	AllowlistTrustThreshold int // auto mode: min successful transactions before add (default 1)
	BlocklistPath           string
	BlocklistURL            string
	BlocklistReload         int    // hours
	AlertWebhookURL         string // optional: POST on rate limit / blocklist
}

func Load() (*Config, error) {
	port, _ := strconv.Atoi(getEnv("PORT", "8080"))
	debug, _ := strconv.ParseBool(getEnv("DEBUG", "false"))

	domain := getEnv("SERVER_DOMAIN", "localhost")
	dbURL := getEnv("DATABASE_URL", "postgres://messenger:messenger_dev@localhost:5432/messenger?sslmode=disable")
	redisURL := getEnv("REDIS_URL", "redis://localhost:6379")
	redisDisabled, _ := strconv.ParseBool(getEnv("REDIS_DISABLED", "true"))

	e2eeStrictOnly, _ := strconv.ParseBool(getEnv("E2EE_STRICT_ONLY", "false"))
	cfg := &Config{
		Server: ServerConfig{
			Domain:         domain,
			Port:           port,
			Debug:          debug,
			E2EEStrictOnly: e2eeStrictOnly,
		},
		Database: DatabaseConfig{
			URL:           dbURL,
			FederationURL: getEnv("DATABASE_FEDERATION_URL", ""),
			EncryptionKey: getEnv("DB_ENCRYPTION_KEY", ""),
		},
		S3: S3Config{
			Endpoint:  getEnv("S3_ENDPOINT", "http://localhost:9000"),
			AccessKey: getEnv("S3_ACCESS_KEY", "minioadmin"),
			SecretKey: getEnv("S3_SECRET_KEY", "minioadmin"),
			Bucket:    getEnv("S3_BUCKET", "messenger-media"),
			UseSSL:    getEnv("S3_USE_SSL", "false") == "true",
			Region:    getEnv("S3_REGION", "us-east-1"),
		},
		Redis: RedisConfig{
			URL:      redisURL,
			Disabled: redisDisabled,
		},
		Federation: FederationConfig{
			Endpoint:   fmt.Sprintf("https://%s/federation", domain),
			Enabled:    true,
			Peers:      splitTrim(getEnv("FEDERATION_PEERS", ""), ","),
			Mode:       getEnv("FEDERATION_MODE", "open"),
			MainDomain: getEnv("FEDERATION_MAIN_DOMAIN", ""),
			MTLS: FederationMTLSConfig{
				ClientCert: getEnv("FEDERATION_MTLS_CLIENT_CERT", ""),
				ClientKey:  getEnv("FEDERATION_MTLS_CLIENT_KEY", ""),
			},
			Security: FederationSecurityConfig{
				RateLimit:               atoiEnv("FEDERATION_RATE_LIMIT", 100),
				MaxBodySize:             atoiEnv("FEDERATION_MAX_BODY_SIZE", 1048576), // 1MB
				AllowlistMode:           getEnv("FEDERATION_ALLOWLIST_MODE", "auto"),
				AllowlistPath:           getEnv("FEDERATION_ALLOWLIST_PATH", ""),
				AllowlistTrustThreshold: atoiEnv("FEDERATION_ALLOWLIST_TRUST_THRESHOLD", 1),
				BlocklistPath:           getEnv("FEDERATION_BLOCKLIST_PATH", ""),
				BlocklistURL:            getEnv("FEDERATION_BLOCKLIST_URL", ""),
				BlocklistReload:         atoiEnv("FEDERATION_BLOCKLIST_RELOAD_HOURS", 6),
				AlertWebhookURL:         getEnv("FEDERATION_ALERT_WEBHOOK_URL", ""),
			},
		},
		Push: PushConfig{
			VAPIDPublicKey:  getEnv("PUSH_VAPID_PUBLIC_KEY", ""),
			VAPIDPrivateKey: getEnv("PUSH_VAPID_PRIVATE_KEY", ""),
		},
		VPN: VPNConfig{
			Enabled:           getEnv("VPN_ENABLED", "false") == "true",
			DefaultExpiryDays: atoi(getEnv("VPN_DEFAULT_EXPIRY_DAYS", "30")),
			NodesJSON:         getEnv("VPN_NODES_JSON", ""),
			WireGuard: WireGuardVPNConfig{
				Enabled:        getEnv("VPN_WIREGUARD_ENABLED", "true") == "true",
				Endpoint:       getEnv("VPN_WIREGUARD_ENDPOINT", "localhost:51820"),
				ServerPubKey:   getEnv("VPN_WIREGUARD_SERVER_PUBLIC_KEY", ""),
				ClientSubnet:   getEnv("VPN_WIREGUARD_CLIENT_SUBNET", "10.66.66.0/24"),
				StatsInterface: getEnv("VPN_WIREGUARD_STATS_INTERFACE", ""),
			},
			OpenVPNTCP443: OpenVPNVPNConfig{
				Enabled:  getEnv("VPN_OPENVPN_TCP443_ENABLED", "true") == "true",
				Endpoint: getEnv("VPN_OPENVPN_ENDPOINT", "localhost:443"),
			},
			Xray: XrayVPNConfig{
				Enabled:    getEnv("VPN_XRAY_ENABLED", "false") == "true",
				Endpoint:   getEnv("VPN_XRAY_ENDPOINT", "localhost:443"),
				APIAddr:    getEnv("VPN_XRAY_API_ADDR", ""),
				VmessPort:  atoiPort(getEnv("VPN_XRAY_VMESS_PORT", "0")),
				VlessPort:  atoiPort(getEnv("VPN_XRAY_VLESS_PORT", "0")),
				TrojanPort: atoiPort(getEnv("VPN_XRAY_TROJAN_PORT", "0")),
			},
		},
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	if n <= 0 {
		return 30
	}
	return n
}

func atoiEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func atoiPort(s string) int {
	n, _ := strconv.Atoi(s)
	if n < 0 {
		return 0
	}
	return n
}
