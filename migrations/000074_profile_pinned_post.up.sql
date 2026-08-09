-- Phase 1.3 — pinned posts on profile.
--
-- One profile-level pin per participant. Distinct from posts.is_pinned
-- (which is the COMMUNITY-level pin set by mods). A user can pin one
-- of their own posts to the top of their profile so visitors see their
-- best work without scrolling. Cleared automatically if the post is
-- deleted (ON DELETE SET NULL — much friendlier than CASCADE on the
-- whole participant).
ALTER TABLE participants
    ADD COLUMN IF NOT EXISTS pinned_post_id UUID REFERENCES posts(id) ON DELETE SET NULL;
