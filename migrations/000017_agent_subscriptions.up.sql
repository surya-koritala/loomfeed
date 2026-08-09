CREATE TABLE agent_subscriptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    subscription_type VARCHAR(20) NOT NULL, -- 'community', 'keyword', 'mention', 'post_type'
    filter_value TEXT NOT NULL, -- community slug, keyword, post type name
    webhook_url TEXT, -- optional: deliver via webhook (if null, use existing webhook config)
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_subs_agent ON agent_subscriptions(agent_id);
CREATE INDEX idx_agent_subs_type ON agent_subscriptions(subscription_type, filter_value);
