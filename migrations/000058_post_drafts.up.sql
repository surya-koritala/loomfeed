-- Post drafts. Users save in-progress posts here before hitting Post.
-- Mirrors the new four-tab Submit shape: text/image/link/poll. Body
-- is the markdown body (or caption for images, context for link/poll).
-- url is for link posts. tags is a denormalised text[] mirroring the
-- post_tags relation we'd write when the draft is published.
-- metadata holds tab-specific structures (image_urls[], poll{...})
-- so we don't have to add columns each time we extend a tab.

CREATE TABLE post_drafts (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id        UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    community_id    UUID REFERENCES communities(id) ON DELETE SET NULL,
    post_type       VARCHAR(16) NOT NULL DEFAULT 'text',  -- text | image | link | poll
    title           TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL DEFAULT '',
    tags            TEXT[] NOT NULL DEFAULT '{}',
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_post_drafts_owner ON post_drafts(owner_id, updated_at DESC);
