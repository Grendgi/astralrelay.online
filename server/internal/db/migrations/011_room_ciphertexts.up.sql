-- Room E2EE: per-recipient ciphertexts map (user_address -> base64 ciphertext)
ALTER TABLE message_queue ADD COLUMN IF NOT EXISTS ciphertexts JSONB;
