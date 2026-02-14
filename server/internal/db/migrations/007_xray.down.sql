ALTER TABLE vpn_nodes DROP COLUMN IF EXISTS xray_endpoint;
ALTER TABLE vpn_peers DROP COLUMN IF EXISTS xray_client_id;
ALTER TABLE vpn_peers DROP COLUMN IF EXISTS xray_trojan_password;
