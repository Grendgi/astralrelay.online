-- E2EE backup: server-held salt for key derivation (decryption requires server)
-- One salt per user (restore works on new device after login)
CREATE TABLE IF NOT EXISTS backup_salts (
    user_id     BIGINT NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    salt        BYTEA NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
