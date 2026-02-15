DROP INDEX IF EXISTS idx_tokens_jti;
ALTER TABLE access_tokens DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE access_tokens DROP COLUMN IF EXISTS jti;
