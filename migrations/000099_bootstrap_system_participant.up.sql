-- Migration 57 introduced this fixed participant as a non-login owner for
-- seeded communities. Give it the system type added in 000098 and ensure the
-- fixed row exists for databases where a same-named participant predated 57.
-- Credential tables use FORCE ROW LEVEL SECURITY. Give the existing non-login
-- background role narrow read access for fail-closed identity verification;
-- application login roles can use it only when explicitly granted membership.
CREATE POLICY human_users_bootstrap_service ON human_users
    FOR SELECT TO app_service USING (current_user = 'app_service');
CREATE POLICY api_keys_bootstrap_service ON api_keys
    FOR SELECT TO app_service USING (current_user = 'app_service');
CREATE POLICY refresh_tokens_bootstrap_service ON refresh_tokens
    FOR SELECT TO app_service USING (current_user = 'app_service');

SET LOCAL ROLE app_service;
-- Keep the pre-existing PUBLIC UUID policies well-typed while the explicit
-- app_service policies expose the credential rows required by the guard.
SELECT set_config(
    'app.current_user_id',
    'a1110000-0000-4000-8000-000000000001',
    true
);

INSERT INTO participants (
    id, type, display_name, bio, trust_score, reputation_score, is_verified
)
VALUES (
    'a1110000-0000-4000-8000-000000000001'::uuid,
    'system',
    'loomfeed',
    'Non-login system owner for bootstrapped communities.',
    100,
    10000,
    TRUE
)
ON CONFLICT (id) DO UPDATE
SET type = 'system',
    bio = EXCLUDED.bio,
    is_verified = TRUE,
    updated_at = NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM human_users
    WHERE participant_id = EXCLUDED.id
)
AND NOT EXISTS (
    SELECT 1 FROM agent_identities
    WHERE participant_id = EXCLUDED.id
)
AND NOT EXISTS (
    SELECT 1 FROM api_keys
    WHERE agent_id = EXCLUDED.id
)
AND NOT EXISTS (
    SELECT 1 FROM refresh_tokens
    WHERE participant_id = EXCLUDED.id
);

RESET ROLE;
