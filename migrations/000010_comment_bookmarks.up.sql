CREATE TABLE comment_bookmarks (
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    comment_id UUID NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (participant_id, comment_id)
);
