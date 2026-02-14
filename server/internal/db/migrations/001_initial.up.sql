-- Accounts: users and devices (multi-device via password login)
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    username        TEXT NOT NULL,
    domain          TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_users_domain_username ON users(domain, username);

-- Devices: MVP = 1 device per user
CREATE TABLE devices (
    id              UUID PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    identity_key    BYTEA NOT NULL,
    signed_prekey   BYTEA NOT NULL,
    signed_prekey_sig BYTEA NOT NULL,
    signed_prekey_id BIGINT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_devices_user ON devices(user_id);

-- One-time prekeys
CREATE TABLE one_time_prekeys (
    id          BIGSERIAL PRIMARY KEY,
    device_id   UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    key_id      BIGINT NOT NULL,
    prekey      BYTEA NOT NULL,
    consumed_at TIMESTAMPTZ,
    UNIQUE(device_id, key_id)
);

CREATE INDEX idx_prekeys_device_unconsumed ON one_time_prekeys(device_id) WHERE consumed_at IS NULL;

-- Access tokens (JWT can be stateless; this table for revocation / optional lookup)
CREATE TABLE access_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id   UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    token_hash  BYTEA NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tokens_hash ON access_tokens(token_hash);
CREATE INDEX idx_tokens_expires ON access_tokens(expires_at);

-- Message queue (delivery)
CREATE TABLE message_queue (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id    TEXT NOT NULL UNIQUE,
    sender      TEXT NOT NULL,
    recipient   TEXT NOT NULL,
    sender_device TEXT NOT NULL,
    msg_type    TEXT NOT NULL DEFAULT 'm.room.encrypted',
    ciphertext  BYTEA NOT NULL,
    session_id  TEXT NOT NULL,
    timestamp   BIGINT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'queued',
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_queue_recipient_status ON message_queue(recipient, status);
CREATE INDEX idx_queue_expires ON message_queue(expires_at);
CREATE INDEX idx_queue_sender ON message_queue(sender);

-- Sync cursors (for long-poll continuation)
CREATE TABLE sync_cursors (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id   UUID NOT NULL,
    cursor      TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, device_id)
);

-- Idempotency (for duplicate send prevention)
CREATE TABLE idempotency_keys (
    key_hash    BYTEA PRIMARY KEY,
    event_id    TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Federation: server signing key (stored as config, not in migrations - see app init)
CREATE TABLE federation_peers (
    domain      TEXT PRIMARY KEY,
    endpoint    TEXT NOT NULL,
    server_key  BYTEA,
    allowed     BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
