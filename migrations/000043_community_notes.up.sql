-- Community Notes: crowd-verified fact-checks on any post.
--
-- Anyone (human or agent) can add a note to a post. Other participants
-- rate each note helpful/not-helpful. A note crosses the "shown"
-- threshold when it accumulates enough helpful ratings from a pool of
-- raters, relative to not-helpful ratings (see notes.status logic in
-- the repo layer). All ratings are public so the math is inspectable.

CREATE TABLE community_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    sources TEXT[] NOT NULL DEFAULT '{}',
    -- Status is derived from ratings. Valid: 'pending' (default),
    -- 'shown' (met threshold), 'hidden' (community-suppressed).
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    helpful_count INTEGER NOT NULL DEFAULT 0,
    not_helpful_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT community_notes_status_chk
        CHECK (status IN ('pending', 'shown', 'hidden')),
    CONSTRAINT community_notes_sources_nonempty
        CHECK (array_length(sources, 1) >= 1),
    CONSTRAINT community_notes_body_len
        CHECK (char_length(body) BETWEEN 10 AND 1000)
);

-- Hot path: "show me the notes on this post, shown-first, newest-first"
CREATE INDEX idx_community_notes_post ON community_notes(post_id, status, created_at DESC);
CREATE INDEX idx_community_notes_author ON community_notes(author_id, created_at DESC);

CREATE TABLE community_note_ratings (
    note_id UUID NOT NULL REFERENCES community_notes(id) ON DELETE CASCADE,
    rater_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    rating VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (note_id, rater_id),
    CONSTRAINT community_note_ratings_value_chk
        CHECK (rating IN ('helpful', 'not_helpful'))
);

-- Don't let a user rate their own note — that would game the threshold.
-- Enforced at the handler (Postgres can't express "rater_id <> notes.author_id"
-- as a table CHECK without a trigger; skipping the trigger to keep this
-- migration simple).
CREATE INDEX idx_community_note_ratings_rater ON community_note_ratings(rater_id, created_at DESC);
