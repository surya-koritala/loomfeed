DROP POLICY IF EXISTS messages_select ON messages;
DROP POLICY IF EXISTS messages_insert ON messages;
DROP POLICY IF EXISTS messages_service ON messages;
ALTER TABLE messages DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS conv_participants_select ON conversation_participants;
DROP POLICY IF EXISTS conv_participants_service ON conversation_participants;
ALTER TABLE conversation_participants DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS conversations_select ON conversations;
DROP POLICY IF EXISTS conversations_modify ON conversations;
DROP POLICY IF EXISTS conversations_service ON conversations;
ALTER TABLE conversations DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS human_users_select ON human_users;
DROP POLICY IF EXISTS human_users_update ON human_users;
DROP POLICY IF EXISTS human_users_insert ON human_users;
DROP POLICY IF EXISTS human_users_service ON human_users;
ALTER TABLE human_users DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS api_keys_select_own ON api_keys;
DROP POLICY IF EXISTS api_keys_insert ON api_keys;
DROP POLICY IF EXISTS api_keys_update ON api_keys;
DROP POLICY IF EXISTS api_keys_service ON api_keys;
ALTER TABLE api_keys DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS notifications_select ON notifications;
DROP POLICY IF EXISTS notifications_insert ON notifications;
DROP POLICY IF EXISTS notifications_update ON notifications;
DROP POLICY IF EXISTS notifications_service ON notifications;
ALTER TABLE notifications DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS votes_select ON votes;
DROP POLICY IF EXISTS votes_insert ON votes;
DROP POLICY IF EXISTS votes_modify ON votes;
DROP POLICY IF EXISTS votes_delete ON votes;
DROP POLICY IF EXISTS votes_service ON votes;
ALTER TABLE votes DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS bookmarks_select ON bookmarks;
DROP POLICY IF EXISTS bookmarks_insert ON bookmarks;
DROP POLICY IF EXISTS bookmarks_delete ON bookmarks;
DROP POLICY IF EXISTS bookmarks_service ON bookmarks;
ALTER TABLE bookmarks DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS refresh_tokens_select ON refresh_tokens;
DROP POLICY IF EXISTS refresh_tokens_insert ON refresh_tokens;
DROP POLICY IF EXISTS refresh_tokens_update ON refresh_tokens;
DROP POLICY IF EXISTS refresh_tokens_delete ON refresh_tokens;
DROP POLICY IF EXISTS refresh_tokens_service ON refresh_tokens;
ALTER TABLE refresh_tokens DISABLE ROW LEVEL SECURITY;
