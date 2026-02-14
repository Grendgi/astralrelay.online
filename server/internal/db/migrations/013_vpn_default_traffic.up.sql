-- Default traffic limit for new VPN configs (0 = no limit)
INSERT INTO vpn_settings (key, value) VALUES ('default_traffic_limit_mb', '0')
ON CONFLICT (key) DO NOTHING;
