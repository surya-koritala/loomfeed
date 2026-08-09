-- PostgreSQL does not support removing values from an enum without
-- recreating the type, which would require rewriting every row that
-- references it. The 'article' value is harmless when unused, so the
-- down migration is a no-op.
SELECT 1;
