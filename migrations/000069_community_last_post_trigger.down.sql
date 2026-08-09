DROP TRIGGER IF EXISTS trg_post_delete_community_last ON posts;
DROP TRIGGER IF EXISTS trg_post_softdelete_community_last ON posts;
DROP TRIGGER IF EXISTS trg_post_insert_community_last ON posts;
DROP FUNCTION IF EXISTS update_community_last_post_at();
