-- Index for cleanup: DELETE ... WHERE consumed_at IS NOT NULL AND consumed_at < NOW() - INTERVAL '30 days'
CREATE INDEX idx_prekeys_consumed_at ON one_time_prekeys(consumed_at) WHERE consumed_at IS NOT NULL;
