-- Add jti for revocation lookup and revoked_at
ALTER TABLE access_tokens ADD COLUMN IF NOT EXISTS jti UUID UNIQUE;
ALTER TABLE access_tokens ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_tokens_jti ON access_tokens(jti) WHERE jti IS NOT NULL;
