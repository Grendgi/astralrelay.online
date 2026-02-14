-- VPN nodes (multi-server support)
CREATE TABLE IF NOT EXISTS vpn_nodes (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                    TEXT NOT NULL,
    region                  TEXT DEFAULT '',
    wireguard_endpoint      TEXT DEFAULT '',
    wireguard_server_pubkey TEXT DEFAULT '',
    openvpn_endpoint        TEXT DEFAULT '',
    is_default              BOOLEAN NOT NULL DEFAULT FALSE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One default node per protocol
CREATE UNIQUE INDEX IF NOT EXISTS idx_vpn_nodes_default ON vpn_nodes (is_default) WHERE is_default = TRUE;

-- Link peers to nodes (null = legacy, use env config)
ALTER TABLE vpn_peers ADD COLUMN IF NOT EXISTS node_id UUID REFERENCES vpn_nodes(id) ON DELETE SET NULL;

-- Seed default node (empty = use config from env)
INSERT INTO vpn_nodes (id, name, region, is_default) 
SELECT gen_random_uuid(), 'Default', '', TRUE
WHERE NOT EXISTS (SELECT 1 FROM vpn_nodes WHERE is_default = TRUE);
