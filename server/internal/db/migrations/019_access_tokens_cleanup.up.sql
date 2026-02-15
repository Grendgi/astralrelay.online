-- Index for cleanup query: DELETE ... WHERE revoked_at < NOW() - INTERVAL '7 days'
CREATE INDEX IF NOT EXISTS idx_tokens_revoked_at ON access_tokens(revoked_at) WHERE revoked_at IS NOT NULL;
