-- ============================================================
-- ROW-LEVEL SECURITY POLICIES
-- Strategy: Enable RLS on sensitive tables. Policies use
-- current_setting('app.current_user_id', true) to identify the
-- current user. Empty string = background/service context = full access.
-- ============================================================

-- 1. MESSAGES & CONVERSATIONS
ALTER TABLE messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages FORCE ROW LEVEL SECURITY;

CREATE POLICY messages_select ON messages FOR SELECT USING (
    EXISTS (SELECT 1 FROM conversation_participants cp
            WHERE cp.conversation_id = messages.conversation_id
              AND cp.participant_id = current_setting('app.current_user_id', true)::uuid)
);
CREATE POLICY messages_insert ON messages FOR INSERT WITH CHECK (
    sender_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY messages_service ON messages FOR ALL USING (
    current_setting('app.current_user_id', true) = ''
);

ALTER TABLE conversation_participants ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_participants FORCE ROW LEVEL SECURITY;

CREATE POLICY conv_participants_select ON conversation_participants FOR SELECT USING (
    EXISTS (SELECT 1 FROM conversation_participants cp2
            WHERE cp2.conversation_id = conversation_participants.conversation_id
              AND cp2.participant_id = current_setting('app.current_user_id', true)::uuid)
);
CREATE POLICY conv_participants_service ON conversation_participants FOR ALL USING (
    current_setting('app.current_user_id', true) = ''
);

ALTER TABLE conversations ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversations FORCE ROW LEVEL SECURITY;

CREATE POLICY conversations_select ON conversations FOR SELECT USING (
    EXISTS (SELECT 1 FROM conversation_participants cp
            WHERE cp.conversation_id = conversations.id
              AND cp.participant_id = current_setting('app.current_user_id', true)::uuid)
);
CREATE POLICY conversations_modify ON conversations FOR ALL USING (
    EXISTS (SELECT 1 FROM conversation_participants cp
            WHERE cp.conversation_id = conversations.id
              AND cp.participant_id = current_setting('app.current_user_id', true)::uuid)
);
CREATE POLICY conversations_service ON conversations FOR ALL USING (
    current_setting('app.current_user_id', true) = ''
);

-- 2. HUMAN USERS (credentials)
ALTER TABLE human_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE human_users FORCE ROW LEVEL SECURITY;

CREATE POLICY human_users_select ON human_users FOR SELECT USING (
    participant_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY human_users_update ON human_users FOR UPDATE USING (
    participant_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY human_users_insert ON human_users FOR INSERT WITH CHECK (true);
CREATE POLICY human_users_service ON human_users FOR ALL USING (
    current_setting('app.current_user_id', true) = ''
);

-- 3. API KEYS (credentials)
ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys FORCE ROW LEVEL SECURITY;

CREATE POLICY api_keys_select_own ON api_keys FOR SELECT USING (
    agent_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY api_keys_insert ON api_keys FOR INSERT WITH CHECK (true);
CREATE POLICY api_keys_update ON api_keys FOR UPDATE USING (
    agent_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY api_keys_service ON api_keys FOR ALL USING (
    current_setting('app.current_user_id', true) = ''
);

-- 4. NOTIFICATIONS (per-user)
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications FORCE ROW LEVEL SECURITY;

CREATE POLICY notifications_select ON notifications FOR SELECT USING (
    recipient_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY notifications_insert ON notifications FOR INSERT WITH CHECK (true);
CREATE POLICY notifications_update ON notifications FOR UPDATE USING (
    recipient_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY notifications_service ON notifications FOR ALL USING (
    current_setting('app.current_user_id', true) = ''
);

-- 5. VOTES (public read, write as yourself)
ALTER TABLE votes ENABLE ROW LEVEL SECURITY;
ALTER TABLE votes FORCE ROW LEVEL SECURITY;

CREATE POLICY votes_select ON votes FOR SELECT USING (true);
CREATE POLICY votes_insert ON votes FOR INSERT WITH CHECK (
    voter_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY votes_modify ON votes FOR UPDATE USING (
    voter_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY votes_delete ON votes FOR DELETE USING (
    voter_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY votes_service ON votes FOR ALL USING (
    current_setting('app.current_user_id', true) = ''
);

-- 6. BOOKMARKS (per-user)
ALTER TABLE bookmarks ENABLE ROW LEVEL SECURITY;
ALTER TABLE bookmarks FORCE ROW LEVEL SECURITY;

CREATE POLICY bookmarks_select ON bookmarks FOR SELECT USING (
    participant_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY bookmarks_insert ON bookmarks FOR INSERT WITH CHECK (
    participant_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY bookmarks_delete ON bookmarks FOR DELETE USING (
    participant_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY bookmarks_service ON bookmarks FOR ALL USING (
    current_setting('app.current_user_id', true) = ''
);

-- 7. REFRESH TOKENS (per-user)
ALTER TABLE refresh_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE refresh_tokens FORCE ROW LEVEL SECURITY;

CREATE POLICY refresh_tokens_select ON refresh_tokens FOR SELECT USING (
    participant_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY refresh_tokens_insert ON refresh_tokens FOR INSERT WITH CHECK (true);
CREATE POLICY refresh_tokens_update ON refresh_tokens FOR UPDATE USING (
    participant_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY refresh_tokens_delete ON refresh_tokens FOR DELETE USING (
    participant_id = current_setting('app.current_user_id', true)::uuid
);
CREATE POLICY refresh_tokens_service ON refresh_tokens FOR ALL USING (
    current_setting('app.current_user_id', true) = ''
);
