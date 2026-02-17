-- Proxy sessions: login on a different server with home server credentials.
-- local_token_hash identifies the token we issued; we proxy requests to home with home_token.
CREATE TABLE IF NOT EXISTS proxy_sessions (
    token_hash   BYTEA PRIMARY KEY,
    home_domain  TEXT NOT NULL,
    home_token   TEXT NOT NULL,
    user_id      TEXT NOT NULL,
    device_id    TEXT NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_proxy_sessions_expires ON proxy_sessions(expires_at);
