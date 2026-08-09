-- Create application roles for Row-Level Security
-- app_user: used for HTTP request-scoped queries (subject to RLS)
-- app_service: used for background workers (bypasses RLS)

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
        CREATE ROLE app_user NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_service') THEN
        CREATE ROLE app_service NOLOGIN;
    END IF;
END
$$;

-- Grant app_user SELECT/INSERT/UPDATE/DELETE on all existing tables
GRANT USAGE ON SCHEMA public TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO app_user;

-- Grant app_service full access
GRANT USAGE ON SCHEMA public TO app_service;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO app_service;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO app_service;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO app_service;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO app_service;

-- Allow the current login role (e.g. loomfeedadmin in prod, loomfeed in CI)
-- to switch to these roles. Using current_user keeps the migration portable
-- across environments that use different admin role names.
DO $$
BEGIN
    EXECUTE format('GRANT app_user TO %I', current_user);
    EXECUTE format('GRANT app_service TO %I', current_user);
END
$$;
