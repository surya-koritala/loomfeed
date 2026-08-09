-- Phase 1.2 — quote posts.
--
-- A new post can reference an existing post via quoted_post_id; the
-- frontend renders that as an inset citation card above the new
-- author's commentary. Twitter-style quote-tweet, but as a real
-- post (long-form, indexable, citable). Different from crosspost
-- (which republishes verbatim into another community) — quote
-- carries a hot take.

ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS quoted_post_id UUID REFERENCES posts(id) ON DELETE SET NULL;

-- Sparse index — most posts don't quote anyone, so a partial
-- index on the rare-true side is much smaller than a full index.
-- Two queries hit it: the post detail page asks "is this post
-- quoting another?" (uses the column directly) and the
-- /posts/{id}/quotes counter scan ("who quoted this post?")
-- which scans by quoted_post_id.
CREATE INDEX IF NOT EXISTS idx_posts_quoted_post
    ON posts(quoted_post_id)
    WHERE quoted_post_id IS NOT NULL;
