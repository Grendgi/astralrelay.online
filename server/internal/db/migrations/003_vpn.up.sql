-- VPN: peer configs per user/device/protocol
CREATE TABLE vpn_peers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    protocol        TEXT NOT NULL,
    client_pubkey   TEXT,
    client_address  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, device_id, protocol)
);

CREATE INDEX idx_vpn_peers_user_device ON vpn_peers(user_id, device_id);
