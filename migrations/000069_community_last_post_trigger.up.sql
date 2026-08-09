-- Maintain communities.last_post_at automatically. Trigger fires on
-- post insert and on the soft-delete update (deleted_at flip), so
-- "deleted the only post" correctly rolls last_post_at back to the
-- next-newest live post.
--
-- We use a function that recomputes from posts each time. It's an
-- O(log n) lookup with the existing posts(community_id, created_at)
-- index, far cheaper than maintaining an aggregate-counter pattern.

CREATE OR REPLACE FUNCTION update_community_last_post_at()
RETURNS TRIGGER AS $$
DECLARE
    target_cid UUID;
BEGIN
    -- Pick which community to recompute. On INSERT/UPDATE the NEW
    -- row carries it; on DELETE the OLD row does.
    IF TG_OP = 'DELETE' THEN
        target_cid := OLD.community_id;
    ELSE
        target_cid := NEW.community_id;
    END IF;

    IF target_cid IS NULL THEN
        RETURN NULL;
    END IF;

    UPDATE communities
    SET last_post_at = (
        SELECT MAX(created_at) FROM posts
        WHERE community_id = target_cid AND deleted_at IS NULL
    )
    WHERE id = target_cid;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Insert: new post lands → bump last_post_at.
DROP TRIGGER IF EXISTS trg_post_insert_community_last ON posts;
CREATE TRIGGER trg_post_insert_community_last
AFTER INSERT ON posts
FOR EACH ROW
EXECUTE FUNCTION update_community_last_post_at();

-- Update: when deleted_at flips (soft delete), recompute.
DROP TRIGGER IF EXISTS trg_post_softdelete_community_last ON posts;
CREATE TRIGGER trg_post_softdelete_community_last
AFTER UPDATE OF deleted_at ON posts
FOR EACH ROW
WHEN (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
EXECUTE FUNCTION update_community_last_post_at();

-- Hard delete (rare; covered for completeness).
DROP TRIGGER IF EXISTS trg_post_delete_community_last ON posts;
CREATE TRIGGER trg_post_delete_community_last
AFTER DELETE ON posts
FOR EACH ROW
EXECUTE FUNCTION update_community_last_post_at();
