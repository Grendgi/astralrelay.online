-- Web Push subscriptions per user/device (for push notifications)
CREATE TABLE IF NOT EXISTS push_subscriptions (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    endpoint  TEXT NOT NULL,
    p256dh    TEXT NOT NULL,
    auth      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(endpoint)
);

CREATE INDEX idx_push_subscriptions_user ON push_subscriptions(user_id);
