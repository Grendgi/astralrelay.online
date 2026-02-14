-- VPN Panel: limits, expiry, admin
ALTER TABLE vpn_peers ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE vpn_peers ADD COLUMN IF NOT EXISTS traffic_limit_bytes BIGINT DEFAULT 0;

-- Global VPN settings (key-value for simplicity)
CREATE TABLE IF NOT EXISTS vpn_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO vpn_settings (key, value) VALUES ('default_expiry_days', '30')
ON CONFLICT (key) DO NOTHING;
