ALTER TABLE vpn_peers DROP COLUMN IF EXISTS expires_at;
ALTER TABLE vpn_peers DROP COLUMN IF EXISTS traffic_limit_bytes;
DROP TABLE IF EXISTS vpn_settings;
