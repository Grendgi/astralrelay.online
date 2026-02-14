-- Xray (VMess/VLESS/Trojan) support
ALTER TABLE vpn_nodes ADD COLUMN IF NOT EXISTS xray_endpoint TEXT DEFAULT '';

ALTER TABLE vpn_peers ADD COLUMN IF NOT EXISTS xray_client_id TEXT;
ALTER TABLE vpn_peers ADD COLUMN IF NOT EXISTS xray_trojan_password TEXT;
