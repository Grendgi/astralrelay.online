package vpn

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/messenger/server/internal/config"
	"github.com/messenger/server/internal/db"
)

type ProtocolInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Hint string `json:"hint"`
}

type Service struct {
	cfg     *config.VPNConfig
	db      *db.DB
	nodes   *NodeService
	xrayAPI *XrayAPI
}

func New(cfg *config.VPNConfig, database *db.DB) *Service {
	var xrayAPI *XrayAPI
	if cfg.Xray.Enabled && cfg.Xray.APIAddr != "" {
		var err error
		xrayAPI, err = NewXrayAPI(cfg.Xray.APIAddr)
		if err != nil {
			// log but don't fail — config generation still works without API
			xrayAPI = nil
		}
	}
	return &Service{cfg: cfg, db: database, nodes: NewNodeService(cfg, database), xrayAPI: xrayAPI}
}

// xrayEmail returns a unique email for Xray (used as user id in inbound).
func xrayEmail(userID int64, deviceID uuid.UUID) string {
	return fmt.Sprintf("m:%d:%s", userID, deviceID.String())
}

// xrayPort returns effective port for protocol (override from config or from endpoint).
func (s *Service) xrayPort(protocol, defaultPort string) string {
	switch protocol {
	case "vmess":
		if s.cfg.Xray.VmessPort > 0 {
			return strconv.Itoa(s.cfg.Xray.VmessPort)
		}
	case "vless":
		if s.cfg.Xray.VlessPort > 0 {
			return strconv.Itoa(s.cfg.Xray.VlessPort)
		}
	case "trojan":
		if s.cfg.Xray.TrojanPort > 0 {
			return strconv.Itoa(s.cfg.Xray.TrojanPort)
		}
	}
	return defaultPort
}

func (s *Service) ListProtocols() []ProtocolInfo {
	if !s.cfg.Enabled {
		return nil
	}
	var list []ProtocolInfo
	if s.cfg.WireGuard.Enabled && s.cfg.WireGuard.ServerPubKey != "" {
		list = append(list, ProtocolInfo{
			ID:   "wireguard",
			Name: "WireGuard",
			Hint: "Быстро, рекомендуется. Не работает? Попробуй Xray.",
		})
	}
	if s.cfg.OpenVPNTCP443.Enabled {
		list = append(list, ProtocolInfo{
			ID:   "openvpn-tcp443",
			Name: "OpenVPN (TCP 443)",
			Hint: "Если WireGuard заблокирован — маскируется под HTTPS.",
		})
	}
	if s.cfg.Xray.Enabled && s.cfg.Xray.Endpoint != "" {
		list = append(list,
			ProtocolInfo{ID: "vless", Name: "VLESS (XTLS Vision)", Hint: "Рекомендуется. Xray, v2rayN, Nekoray, v2rayNG."},
			ProtocolInfo{ID: "vmess", Name: "VMess (устаревший)", Hint: "Deprecated, предпочтительно VLESS."},
			ProtocolInfo{ID: "trojan", Name: "Trojan (устаревший)", Hint: "Deprecated, предпочтительно VLESS."},
		)
	}
	return list
}

func (s *Service) GetConfig(ctx context.Context, protocol string, userID int64, deviceID uuid.UUID, nodeID *uuid.UUID) (content string, filename string, err error) {
	if !s.cfg.Enabled {
		return "", "", fmt.Errorf("vpn disabled")
	}
	// Check existing peer expiry - reject if expired
	var expiresAt *time.Time
	var trafficLimit int64
	var clientPubkey *string
	_ = s.db.Pool.QueryRow(ctx,
		`SELECT expires_at, COALESCE(traffic_limit_bytes, 0), client_pubkey FROM vpn_peers WHERE user_id = $1 AND device_id = $2 AND protocol = $3`,
		userID, deviceID, protocol,
	).Scan(&expiresAt, &trafficLimit, &clientPubkey)
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return "", "", fmt.Errorf("vpn config expired, request new one from admin")
	}
	// Check traffic limit (WireGuard only, when stats available)
	if trafficLimit > 0 && protocol == "wireguard" && clientPubkey != nil {
		wgStats := s.FetchWGStats(ctx)
		if wgStats != nil {
			if st, ok := wgStats[*clientPubkey]; ok {
				used := st.RxBytes + st.TxBytes
				if used >= trafficLimit {
					return "", "", fmt.Errorf("traffic limit exceeded (%d bytes used, limit %d)", used, trafficLimit)
				}
			}
		}
	}
	node, err := s.nodes.ResolveNode(ctx, nodeID)
	if err != nil {
		return "", "", err
	}
	switch protocol {
	case "wireguard":
		return s.getWireGuardConfig(ctx, userID, deviceID, node)
	case "openvpn-tcp443":
		return s.getOpenVPNConfig(ctx, userID, deviceID, node)
	case "vmess":
		return s.getVMessConfig(ctx, userID, deviceID, node)
	case "vless":
		return s.getVLESSConfig(ctx, userID, deviceID, node)
	case "trojan":
		return s.getTrojanConfig(ctx, userID, deviceID, node)
	default:
		return "", "", fmt.Errorf("unknown protocol: %s", protocol)
	}
}

func (s *Service) getWireGuardConfig(ctx context.Context, userID int64, deviceID uuid.UUID, node *VPNNode) (string, string, error) {
	endpoint := node.EffectiveWGEndpoint(s.cfg)
	pubKey := node.EffectiveWGServerPubKey(s.cfg)
	if !s.cfg.WireGuard.Enabled || pubKey == "" {
		return "", "", fmt.Errorf("wireguard disabled or not configured")
	}
	privKey, clientPubKey, err := WireGuardKeyPair()
	if err != nil {
		return "", "", err
	}

	var oldPubkey string
	_ = s.db.Pool.QueryRow(ctx,
		`SELECT client_pubkey FROM vpn_peers WHERE user_id = $1 AND device_id = $2 AND protocol = 'wireguard'`,
		userID, deviceID,
	).Scan(&oldPubkey)
	if oldPubkey != "" && oldPubkey != clientPubKey {
		s.RemoveWGPeer(ctx, oldPubkey)
	}

	var clientAddr string
	var existingAddr string
	if err := s.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(client_address::text, '') FROM vpn_peers WHERE user_id = $1 AND device_id = $2 AND protocol = 'wireguard'`,
		userID, deviceID,
	).Scan(&existingAddr); err == nil && existingAddr != "" {
		clientAddr = existingAddr
	} else {
		var n int
		if err := s.db.Pool.QueryRow(ctx, `SELECT nextval('vpn_wg_addr_seq')::int`).Scan(&n); err != nil {
			return "", "", fmt.Errorf("vpn address pool exhausted: %w", err)
		}
		clientAddr = fmt.Sprintf("10.66.66.%d/32", n)
	}

	expiryDays := s.getDefaultExpiryDays(ctx)
	expiresAt := time.Now().AddDate(0, 0, expiryDays)
	trafficLimit := s.getDefaultTrafficLimitBytes(ctx)

	var nodeID *uuid.UUID
	if node.ID != uuid.Nil {
		nodeID = &node.ID
	}
	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO vpn_peers (user_id, device_id, protocol, client_pubkey, client_address, expires_at, traffic_limit_bytes, node_id) VALUES ($1, $2, 'wireguard', $3, $4::inet, $5, $6, $7)
		 ON CONFLICT (user_id, device_id, protocol) DO UPDATE SET client_pubkey = EXCLUDED.client_pubkey, client_address = COALESCE(vpn_peers.client_address, EXCLUDED.client_address), expires_at = EXCLUDED.expires_at, traffic_limit_bytes = EXCLUDED.traffic_limit_bytes, node_id = EXCLUDED.node_id, created_at = NOW()`,
		userID, deviceID, clientPubKey, clientAddr, expiresAt, trafficLimit, nodeID,
	)
	if err != nil {
		return "", "", err
	}

	conf := BuildWireGuardConf(privKey, clientPubKey, pubKey, endpoint, clientAddr)
	return conf, "messenger-wireguard.conf", nil
}

func (s *Service) getOpenVPNConfig(ctx context.Context, userID int64, deviceID uuid.UUID, node *VPNNode) (string, string, error) {
	if !s.cfg.OpenVPNTCP443.Enabled {
		return "", "", fmt.Errorf("openvpn disabled")
	}
	endpoint := node.EffectiveOpenVPNEndpoint(s.cfg)
	// MVP: возвращаем шаблон с инструкцией. Полная генерация .ovpn требует PKI (easy-rsa).
	template := `# OpenVPN TCP 443 — мессенджер
# Установите OpenVPN клиент и замените этот файл на сгенерированный конфиг.
# Админ сервера должен выдать .ovpn через панель OpenVPN.
#
# Endpoint: ` + endpoint + `
# Протокол: TCP
# Порт: 443
`
	return template, "messenger-openvpn.ovpn", nil
}

func (s *Service) getVMessConfig(ctx context.Context, userID int64, deviceID uuid.UUID, node *VPNNode) (string, string, error) {
	if !s.cfg.Xray.Enabled {
		return "", "", fmt.Errorf("xray disabled")
	}
	endpoint := node.EffectiveXrayEndpoint(s.cfg)
	if endpoint == "" {
		return "", "", fmt.Errorf("xray endpoint not configured")
	}
	host, port := parseHostPort(endpoint)

	var clientID string
	err := s.db.Pool.QueryRow(ctx,
		`SELECT xray_client_id FROM vpn_peers WHERE user_id = $1 AND device_id = $2 AND protocol = 'vmess'`,
		userID, deviceID,
	).Scan(&clientID)
	if err != nil || clientID == "" {
		clientID = GenerateXrayUUID()
		expiryDays := s.getDefaultExpiryDays(ctx)
		expiresAt := time.Now().AddDate(0, 0, expiryDays)
		var nodeID *uuid.UUID
		if node.ID != uuid.Nil {
			nodeID = &node.ID
		}
		_, err = s.db.Pool.Exec(ctx,
			`INSERT INTO vpn_peers (user_id, device_id, protocol, xray_client_id, expires_at, node_id) VALUES ($1, $2, 'vmess', $3, $4, $5)
			 ON CONFLICT (user_id, device_id, protocol) DO UPDATE SET xray_client_id = $3, expires_at = $4, node_id = $5, created_at = NOW()`,
			userID, deviceID, clientID, expiresAt, nodeID,
		)
		if err != nil {
			return "", "", err
		}
	}
	if s.xrayAPI != nil {
		_ = s.xrayAPI.AddVMessUser(ctx, clientID, xrayEmail(userID, deviceID))
	}

	port = s.xrayPort("vmess", port)
	url := BuildVMessURL(clientID, host, port, "messenger-"+node.Name)
	return url, "messenger-vmess.txt", nil
}

func (s *Service) getVLESSConfig(ctx context.Context, userID int64, deviceID uuid.UUID, node *VPNNode) (string, string, error) {
	if !s.cfg.Xray.Enabled {
		return "", "", fmt.Errorf("xray disabled")
	}
	endpoint := node.EffectiveXrayEndpoint(s.cfg)
	if endpoint == "" {
		return "", "", fmt.Errorf("xray endpoint not configured")
	}
	host, port := parseHostPort(endpoint)

	var clientID string
	err := s.db.Pool.QueryRow(ctx,
		`SELECT xray_client_id FROM vpn_peers WHERE user_id = $1 AND device_id = $2 AND protocol = 'vless'`,
		userID, deviceID,
	).Scan(&clientID)
	if err != nil || clientID == "" {
		clientID = GenerateXrayUUID()
		expiryDays := s.getDefaultExpiryDays(ctx)
		expiresAt := time.Now().AddDate(0, 0, expiryDays)
		var nodeID *uuid.UUID
		if node.ID != uuid.Nil {
			nodeID = &node.ID
		}
		_, err = s.db.Pool.Exec(ctx,
			`INSERT INTO vpn_peers (user_id, device_id, protocol, xray_client_id, expires_at, node_id) VALUES ($1, $2, 'vless', $3, $4, $5)
			 ON CONFLICT (user_id, device_id, protocol) DO UPDATE SET xray_client_id = $3, expires_at = $4, node_id = $5, created_at = NOW()`,
			userID, deviceID, clientID, expiresAt, nodeID,
		)
		if err != nil {
			return "", "", err
		}
	}
	if s.xrayAPI != nil {
		_ = s.xrayAPI.AddVLESSUser(ctx, clientID, xrayEmail(userID, deviceID))
	}

	port = s.xrayPort("vless", port)
	url := BuildVLESSURL(clientID, host, port, "messenger-"+node.Name)
	return url, "messenger-vless.txt", nil
}

func (s *Service) getTrojanConfig(ctx context.Context, userID int64, deviceID uuid.UUID, node *VPNNode) (string, string, error) {
	if !s.cfg.Xray.Enabled {
		return "", "", fmt.Errorf("xray disabled")
	}
	endpoint := node.EffectiveXrayEndpoint(s.cfg)
	if endpoint == "" {
		return "", "", fmt.Errorf("xray endpoint not configured")
	}
	host, port := parseHostPort(endpoint)

	var password string
	err := s.db.Pool.QueryRow(ctx,
		`SELECT xray_trojan_password FROM vpn_peers WHERE user_id = $1 AND device_id = $2 AND protocol = 'trojan'`,
		userID, deviceID,
	).Scan(&password)
	if err != nil || password == "" {
		password, err = GenerateTrojanPassword()
		if err != nil {
			return "", "", err
		}
		expiryDays := s.getDefaultExpiryDays(ctx)
		expiresAt := time.Now().AddDate(0, 0, expiryDays)
		var nodeID *uuid.UUID
		if node.ID != uuid.Nil {
			nodeID = &node.ID
		}
		_, err = s.db.Pool.Exec(ctx,
			`INSERT INTO vpn_peers (user_id, device_id, protocol, xray_trojan_password, expires_at, node_id) VALUES ($1, $2, 'trojan', $3, $4, $5)
			 ON CONFLICT (user_id, device_id, protocol) DO UPDATE SET xray_trojan_password = $3, expires_at = $4, node_id = $5, created_at = NOW()`,
			userID, deviceID, password, expiresAt, nodeID,
		)
		if err != nil {
			return "", "", err
		}
	}
	if s.xrayAPI != nil {
		_ = s.xrayAPI.AddTrojanUser(ctx, password, xrayEmail(userID, deviceID))
	}

	port = s.xrayPort("trojan", port)
	url := BuildTrojanURL(password, host, port, "messenger-"+node.Name)
	return url, "messenger-trojan.txt", nil
}

// MyConfigInfo is returned for user's own configs list.
type MyConfigInfo struct {
	DeviceID         string     `json:"device_id"`
	Protocol         string     `json:"protocol"`
	NodeName         string     `json:"node_name,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	IsExpired        bool       `json:"is_expired"`
	TrafficRxBytes   int64      `json:"traffic_rx_bytes"`
	TrafficTxBytes   int64      `json:"traffic_tx_bytes"`
	TrafficLimitBytes int64     `json:"traffic_limit_bytes,omitempty"`
}

// ListMyConfigs returns VPN configs for the given user only (all their devices).
func (s *Service) ListMyConfigs(ctx context.Context, userID int64) ([]MyConfigInfo, error) {
	wgStats := s.FetchWGStats(ctx)
	rows, err := s.db.Pool.Query(ctx,
		`SELECT p.device_id, p.protocol, COALESCE(n.name, 'Default'), p.created_at, p.expires_at, p.client_pubkey, COALESCE(p.traffic_limit_bytes, 0)
		 FROM vpn_peers p
		 LEFT JOIN vpn_nodes n ON n.id = p.node_id
		 WHERE p.user_id = $1
		 ORDER BY p.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []MyConfigInfo
	for rows.Next() {
		var devID uuid.UUID
		var c MyConfigInfo
		var expiresAt *time.Time
		var clientPubkey *string
		if err := rows.Scan(&devID, &c.Protocol, &c.NodeName, &c.CreatedAt, &expiresAt, &clientPubkey, &c.TrafficLimitBytes); err != nil {
			continue
		}
		c.DeviceID = devID.String()
		c.ExpiresAt = expiresAt
		c.IsExpired = expiresAt != nil && expiresAt.Before(time.Now())
		if clientPubkey != nil && wgStats != nil {
			if st, ok := wgStats[*clientPubkey]; ok {
				c.TrafficRxBytes = st.RxBytes
				c.TrafficTxBytes = st.TxBytes
			}
		}
		list = append(list, c)
	}
	return list, nil
}

// Revoke deletes a VPN config. Caller must ensure it's the peer's own config.
func (s *Service) Revoke(ctx context.Context, userID int64, deviceID uuid.UUID, protocol string) error {
	if s.xrayAPI != nil {
		email := xrayEmail(userID, deviceID)
		switch protocol {
		case "vmess":
			_ = s.xrayAPI.RemoveUser(ctx, inboundTagVMess, email)
		case "vless":
			_ = s.xrayAPI.RemoveUser(ctx, inboundTagVLESS, email)
		case "trojan":
			_ = s.xrayAPI.RemoveUser(ctx, inboundTagTrojan, email)
		}
	}
	_, err := s.db.Pool.Exec(ctx,
		`DELETE FROM vpn_peers WHERE user_id = $1 AND device_id = $2 AND protocol = $3`,
		userID, deviceID, protocol,
	)
	return err
}

// Nodes returns the node service.
func (s *Service) Nodes() *NodeService {
	return s.nodes
}

// getDefaultExpiryDays returns expiry days from vpn_settings or config.
func (s *Service) getDefaultExpiryDays(ctx context.Context) int {
	var v string
	if err := s.db.Pool.QueryRow(ctx, `SELECT value FROM vpn_settings WHERE key = 'default_expiry_days'`).Scan(&v); err == nil {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			return d
		}
	}
	if s.cfg.DefaultExpiryDays > 0 {
		return s.cfg.DefaultExpiryDays
	}
	return 30
}

// getDefaultTrafficLimitBytes returns traffic limit from vpn_settings (0 = no limit). Value in MB.
func (s *Service) getDefaultTrafficLimitBytes(ctx context.Context) int64 {
	var v string
	if err := s.db.Pool.QueryRow(ctx, `SELECT value FROM vpn_settings WHERE key = 'default_traffic_limit_mb'`).Scan(&v); err != nil {
		return 0
	}
	mb, err := strconv.ParseInt(v, 10, 64)
	if err != nil || mb <= 0 {
		return 0
	}
	return mb * 1024 * 1024
}

