package vpn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/messenger/server/internal/config"
	"github.com/messenger/server/internal/db"
)

// VPNNode represents a VPN server node (WireGuard/OpenVPN/Xray).
type VPNNode struct {
	ID                    uuid.UUID `json:"id"`
	Name                  string    `json:"name"`
	Region                string    `json:"region"`
	WireGuardEndpoint     string    `json:"wireguard_endpoint"`
	WireGuardServerPubKey string    `json:"wireguard_server_pubkey"`
	OpenVPNEndpoint       string    `json:"openvpn_endpoint"`
	XrayEndpoint          string    `json:"xray_endpoint"`
	IsDefault             bool      `json:"is_default"`
	PingURL               string    `json:"ping_url,omitempty"` // derived for client-side latency check
}

// derivePingURL extracts host from endpoint and returns https://host/ for latency measurement.
func derivePingURL(wgEndpoint, ovpnEndpoint, xrayEndpoint string) string {
	for _, ep := range []string{xrayEndpoint, ovpnEndpoint, wgEndpoint} {
		if ep == "" {
			continue
		}
		host := ep
		if idx := strings.LastIndex(ep, ":"); idx > 0 {
			host = ep[:idx]
		}
		host = strings.TrimSpace(host)
		if host != "" && !strings.HasPrefix(host, "[") {
			return "https://" + host + "/"
		}
	}
	return ""
}

// NodeService handles vpn_nodes CRUD.
type NodeService struct {
	cfg  *config.VPNConfig
	db   *db.DB
}

// NewNodeService creates a NodeService.
func NewNodeService(cfg *config.VPNConfig, database *db.DB) *NodeService {
	return &NodeService{cfg: cfg, db: database}
}

// ListNodes returns all VPN nodes.
func (ns *NodeService) ListNodes(ctx context.Context) ([]VPNNode, error) {
	rows, err := ns.db.Pool.Query(ctx,
		`SELECT id, name, region, wireguard_endpoint, wireguard_server_pubkey, openvpn_endpoint, COALESCE(xray_endpoint,''), is_default
		 FROM vpn_nodes ORDER BY is_default DESC, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []VPNNode
	for rows.Next() {
		var n VPNNode
		if err := rows.Scan(&n.ID, &n.Name, &n.Region, &n.WireGuardEndpoint, &n.WireGuardServerPubKey, &n.OpenVPNEndpoint, &n.XrayEndpoint, &n.IsDefault); err != nil {
			continue
		}
		n.PingURL = derivePingURL(n.WireGuardEndpoint, n.OpenVPNEndpoint, n.XrayEndpoint)
		list = append(list, n)
	}
	return list, nil
}

// GetNode returns a node by ID, or nil if not found.
func (ns *NodeService) GetNode(ctx context.Context, id uuid.UUID) (*VPNNode, error) {
	var n VPNNode
	err := ns.db.Pool.QueryRow(ctx,
		`SELECT id, name, region, wireguard_endpoint, wireguard_server_pubkey, openvpn_endpoint, COALESCE(xray_endpoint,''), is_default
		 FROM vpn_nodes WHERE id = $1`,
		id,
	).Scan(&n.ID, &n.Name, &n.Region, &n.WireGuardEndpoint, &n.WireGuardServerPubKey, &n.OpenVPNEndpoint, &n.XrayEndpoint, &n.IsDefault)
	if err != nil {
		return nil, err
	}
	n.PingURL = derivePingURL(n.WireGuardEndpoint, n.OpenVPNEndpoint, n.XrayEndpoint)
	return &n, nil
}

// GetDefaultNode returns the default node, or the first node, or a synthetic default from config.
func (ns *NodeService) GetDefaultNode(ctx context.Context) (*VPNNode, error) {
	rows, err := ns.db.Pool.Query(ctx,
		`SELECT id, name, region, wireguard_endpoint, wireguard_server_pubkey, openvpn_endpoint, COALESCE(xray_endpoint,''), is_default
		 FROM vpn_nodes ORDER BY is_default DESC NULLS LAST LIMIT 1`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		var n VPNNode
		if err := rows.Scan(&n.ID, &n.Name, &n.Region, &n.WireGuardEndpoint, &n.WireGuardServerPubKey, &n.OpenVPNEndpoint, &n.XrayEndpoint, &n.IsDefault); err != nil {
			return nil, err
		}
		n.PingURL = derivePingURL(n.WireGuardEndpoint, n.OpenVPNEndpoint, n.XrayEndpoint)
		return &n, nil
	}
	// No nodes in DB: use config as synthetic default
	n := &VPNNode{
		ID:                    uuid.Nil,
		Name:                  "Default",
		Region:                "",
		WireGuardEndpoint:     ns.cfg.WireGuard.Endpoint,
		WireGuardServerPubKey: ns.cfg.WireGuard.ServerPubKey,
		OpenVPNEndpoint:       ns.cfg.OpenVPNTCP443.Endpoint,
		XrayEndpoint:          ns.cfg.Xray.Endpoint,
		IsDefault:             true,
	}
	n.PingURL = derivePingURL(n.WireGuardEndpoint, n.OpenVPNEndpoint, n.XrayEndpoint)
	return n, nil
}

// ResolveNode returns the node for config generation. Prefers nodeID if valid; else default.
func (ns *NodeService) ResolveNode(ctx context.Context, nodeID *uuid.UUID) (*VPNNode, error) {
	if nodeID != nil && *nodeID != uuid.Nil {
		n, err := ns.GetNode(ctx, *nodeID)
		if err == nil {
			return n, nil
		}
	}
	return ns.GetDefaultNode(ctx)
}

// EffectiveWGEndpoint returns the WireGuard endpoint for a node (node overrides config).
func (n *VPNNode) EffectiveWGEndpoint(cfg *config.VPNConfig) string {
	if n.WireGuardEndpoint != "" {
		return n.WireGuardEndpoint
	}
	return cfg.WireGuard.Endpoint
}

// EffectiveWGServerPubKey returns the WireGuard server public key.
func (n *VPNNode) EffectiveWGServerPubKey(cfg *config.VPNConfig) string {
	if n.WireGuardServerPubKey != "" {
		return n.WireGuardServerPubKey
	}
	return cfg.WireGuard.ServerPubKey
}

// EffectiveOpenVPNEndpoint returns the OpenVPN endpoint.
func (n *VPNNode) EffectiveOpenVPNEndpoint(cfg *config.VPNConfig) string {
	if n.OpenVPNEndpoint != "" {
		return n.OpenVPNEndpoint
	}
	return cfg.OpenVPNTCP443.Endpoint
}

// EffectiveXrayEndpoint returns the Xray endpoint.
func (n *VPNNode) EffectiveXrayEndpoint(cfg *config.VPNConfig) string {
	if n.XrayEndpoint != "" {
		return n.XrayEndpoint
	}
	return cfg.Xray.Endpoint
}

// CreateNode inserts a new node.
func (ns *NodeService) CreateNode(ctx context.Context, name, region, wgEndpoint, wgPubKey, ovpnEndpoint string, isDefault bool) (VPNNode, error) {
	if isDefault {
		_, _ = ns.db.Pool.Exec(ctx, `UPDATE vpn_nodes SET is_default = FALSE`)
	}
	var n VPNNode
	err := ns.db.Pool.QueryRow(ctx,
		`INSERT INTO vpn_nodes (name, region, wireguard_endpoint, wireguard_server_pubkey, openvpn_endpoint, is_default)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, region, wireguard_endpoint, wireguard_server_pubkey, openvpn_endpoint, is_default`,
		name, region, wgEndpoint, wgPubKey, ovpnEndpoint, isDefault,
	).Scan(&n.ID, &n.Name, &n.Region, &n.WireGuardEndpoint, &n.WireGuardServerPubKey, &n.OpenVPNEndpoint, &n.IsDefault)
	return n, err
}

// UpdateNode updates a node.
func (ns *NodeService) UpdateNode(ctx context.Context, id uuid.UUID, name, region, wgEndpoint, wgPubKey, ovpnEndpoint string, isDefault bool) error {
	if isDefault {
		_, _ = ns.db.Pool.Exec(ctx, `UPDATE vpn_nodes SET is_default = FALSE WHERE id != $1`, id)
	}
	_, err := ns.db.Pool.Exec(ctx,
		`UPDATE vpn_nodes SET name=$2, region=$3, wireguard_endpoint=$4, wireguard_server_pubkey=$5, openvpn_endpoint=$6, is_default=$7 WHERE id=$1`,
		id, name, region, wgEndpoint, wgPubKey, ovpnEndpoint, isDefault,
	)
	return err
}

// SeedFromJSON upserts nodes from JSON array. Used at startup with VPN_NODES_JSON.
// Format: [{"name":"Amsterdam","region":"EU","wireguard_endpoint":"host:51820","wireguard_server_pubkey":"base64","openvpn_endpoint":"host:443","is_default":false}]
func (ns *NodeService) SeedFromJSON(ctx context.Context, jsonStr string) error {
	if jsonStr == "" {
		return nil
	}
	var nodes []struct {
		Name                  string `json:"name"`
		Region                string `json:"region"`
		WireGuardEndpoint     string `json:"wireguard_endpoint"`
		WireGuardServerPubKey string `json:"wireguard_server_pubkey"`
		OpenVPNEndpoint       string `json:"openvpn_endpoint"`
		XrayEndpoint          string `json:"xray_endpoint"`
		IsDefault             bool   `json:"is_default"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &nodes); err != nil {
		return fmt.Errorf("vpn nodes json: %w", err)
	}
	for _, n := range nodes {
		if n.Name == "" {
			continue
		}
		if n.IsDefault {
			_, _ = ns.db.Pool.Exec(ctx, `UPDATE vpn_nodes SET is_default = FALSE`)
		}
		_, err := ns.db.Pool.Exec(ctx,
			`INSERT INTO vpn_nodes (name, region, wireguard_endpoint, wireguard_server_pubkey, openvpn_endpoint, xray_endpoint, is_default)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (name) DO UPDATE SET region=$2, wireguard_endpoint=$3, wireguard_server_pubkey=$4, openvpn_endpoint=$5, xray_endpoint=$6, is_default=$7`,
			n.Name, n.Region, n.WireGuardEndpoint, n.WireGuardServerPubKey, n.OpenVPNEndpoint, n.XrayEndpoint, n.IsDefault,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// DeleteNode removes a node. Fails if it has peers.
func (ns *NodeService) DeleteNode(ctx context.Context, id uuid.UUID) error {
	var count int
	if err := ns.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM vpn_peers WHERE node_id = $1`, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("cannot delete node with %d peers", count)
	}
	_, err := ns.db.Pool.Exec(ctx, `DELETE FROM vpn_nodes WHERE id = $1`, id)
	return err
}
