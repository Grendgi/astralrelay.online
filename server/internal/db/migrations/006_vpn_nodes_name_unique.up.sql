-- Allow upsert by name for node seeding from VPN_NODES_JSON
CREATE UNIQUE INDEX IF NOT EXISTS idx_vpn_nodes_name ON vpn_nodes (name);
