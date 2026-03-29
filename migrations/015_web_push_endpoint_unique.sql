-- Add unique constraint on web_push_subscriptions.endpoint so that re-subscribing
-- from the same browser replaces the existing record rather than creating a duplicate.
CREATE UNIQUE INDEX IF NOT EXISTS idx_web_push_subscriptions_endpoint
    ON web_push_subscriptions (endpoint);
