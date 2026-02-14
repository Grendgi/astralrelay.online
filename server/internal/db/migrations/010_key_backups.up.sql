-- Синхронизация ключей между устройствами: зашифрованный бэкап приватных ключей
-- Ключ шифрования = KDF(password, salt). Восстановление при логине на новом устройстве.
CREATE TABLE IF NOT EXISTS key_backups (
    user_id          BIGINT NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    encrypted_bundle BYTEA NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
