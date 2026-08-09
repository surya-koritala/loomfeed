-- Community moderation UI — per-community ban list + mod action log
-- + distinguishing moderator-removed content from author-deleted.
--
-- `deleted_at` already marks any hide (author delete OR mod remove). We
-- add `removed_by_id` / `removed_reason` so the UI can show "removed by
-- moderator" separately from a self-deletion, and so the mod log can
-- link back to the action that caused it.

-- Per-community bans. A ban prevents the participant from posting or
-- commenting in that community; it does not delete their past content.
CREATE TABLE community_bans (
    community_id   UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    banned_by_id   UUID NOT NULL REFERENCES participants(id),
    reason         TEXT NOT NULL DEFAULT '',
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (community_id, participant_id)
);

CREATE INDEX idx_community_bans_community ON community_bans(community_id, created_at DESC);
CREATE INDEX idx_community_bans_participant ON community_bans(participant_id);

-- Unified mod log. Every moderator action lands here — approve/remove
-- post, remove comment, ban/unban, add/remove mod. `target_type` +
-- `target_id` identifies what was acted on; `reason` is optional.
CREATE TABLE moderation_actions (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    actor_id     UUID NOT NULL REFERENCES participants(id),
    action       VARCHAR(32) NOT NULL,
    target_type  VARCHAR(16) NOT NULL,
    target_id    UUID NOT NULL,
    reason       TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_moderation_actions_community ON moderation_actions(community_id, created_at DESC);
CREATE INDEX idx_moderation_actions_target ON moderation_actions(target_type, target_id);

-- Track WHO removed content (not just that it was deleted). When set,
-- the row was hidden by a moderator, not by the author.
ALTER TABLE posts    ADD COLUMN removed_by_id UUID REFERENCES participants(id);
ALTER TABLE posts    ADD COLUMN removed_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE comments ADD COLUMN removed_by_id UUID REFERENCES participants(id);
ALTER TABLE comments ADD COLUMN removed_reason TEXT NOT NULL DEFAULT '';
