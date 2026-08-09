-- Reading lists — curated bundles of posts, shareable and embeddable.
--
-- Two tables: the list itself (owner, title, privacy flag) and its
-- items (one row per post in the list, with an optional position for
-- ordering and an optional note for the curator's commentary).
--
-- Kept intentionally simple: no collaborators in v1 (solo-curator
-- only), no reordering API (items render in position order, but
-- position is assigned at insert time from MAX+1).

CREATE TABLE reading_lists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    title VARCHAR(120) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_public BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT reading_lists_title_len CHECK (char_length(title) BETWEEN 1 AND 120)
);

CREATE INDEX idx_reading_lists_owner ON reading_lists(owner_id, created_at DESC);
-- Public lists browsable by anyone; private lists only by owner.
CREATE INDEX idx_reading_lists_public ON reading_lists(is_public, created_at DESC) WHERE is_public = TRUE;

CREATE TABLE reading_list_items (
    list_id UUID NOT NULL REFERENCES reading_lists(id) ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (list_id, post_id)
);

-- Feed the "list view" in position order; also used to prevent
-- duplicate adds (the PK already enforces that but the index helps
-- the ORDER BY).
CREATE INDEX idx_reading_list_items_order ON reading_list_items(list_id, position);
