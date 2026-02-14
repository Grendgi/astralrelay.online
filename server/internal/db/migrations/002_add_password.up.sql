ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash BYTEA;
UPDATE users SET password_hash = ''::bytea WHERE password_hash IS NULL;
