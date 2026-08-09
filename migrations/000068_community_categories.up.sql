-- Communities get two new attributes for the redesigned discovery
-- page: a `category` for grouping (Tech, Science, Lifestyle, etc.)
-- and `last_post_at` for sorting trending/active communities and
-- detecting cold ones.
--
-- Categories use a fixed enum-style VARCHAR — small, indexable, easy
-- to extend without migrations. We default to 'other' so new
-- communities created against the old API surface don't fail; the
-- updated handler will require it explicitly.

ALTER TABLE communities
    ADD COLUMN IF NOT EXISTS category VARCHAR(40) NOT NULL DEFAULT 'other',
    ADD COLUMN IF NOT EXISTS last_post_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_communities_category ON communities(category);
CREATE INDEX IF NOT EXISTS idx_communities_last_post_at ON communities(last_post_at DESC NULLS LAST);

-- Backfill categories for the existing 64 communities. We map each
-- known slug to its bucket. Anything not listed stays 'other' — the
-- redesigned UI surfaces 'other' as a fallback group.
UPDATE communities SET category = 'tech' WHERE slug IN (
    'robotics', 'hardware', 'devops', 'security', 'cryptography',
    'mobile-dev', 'web-dev', 'databases', 'distributed-systems',
    'code-review', 'frameworks', 'machine-learning', 'osai',
    'ai-safety', 'ai-news'
);

UPDATE communities SET category = 'science' WHERE slug IN (
    'biotech', 'neuroscience', 'space', 'environment', 'climate',
    'physics', 'chemistry', 'mathematics', 'research', 'health',
    'food'
);

UPDATE communities SET category = 'culture' WHERE slug IN (
    'philosophy', 'history', 'books', 'music', 'film', 'gaming',
    'visual-art', 'writing', 'sports'
);

UPDATE communities SET category = 'society' WHERE slug IN (
    'privacy', 'ethics', 'education', 'world-news', 'debates',
    'legal', 'culture'
);

UPDATE communities SET category = 'lifestyle' WHERE slug IN (
    'cooking', 'fitness', 'nostalgia', 'travel', 'parenting',
    'gardening', 'diy', 'productivity', 'personal-finance'
);

UPDATE communities SET category = 'mind' WHERE slug IN (
    'psychology', 'life', 'linguistics'
);

UPDATE communities SET category = 'business' WHERE slug IN (
    'careers', 'startups', 'economics', 'finance'
);

UPDATE communities SET category = 'meta' WHERE slug IN (
    'meta', 'general', 'open-forum', 'weird', 'futurism'
);

-- Backfill last_post_at from posts so existing communities have a
-- correct value before the trigger takes over for future writes.
UPDATE communities c
SET last_post_at = (
    SELECT MAX(p.created_at) FROM posts p
    WHERE p.community_id = c.id AND p.deleted_at IS NULL
);
