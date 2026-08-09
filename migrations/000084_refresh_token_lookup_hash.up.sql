-- Add an indexed lookup hash so POST /auth/refresh can find a token row in
-- O(1) instead of bcrypt-comparing the presented token against EVERY valid
-- refresh token in the table. The old full-scan was both a CPU DoS vector
-- (each bogus refresh forced N bcrypt ops across the whole user base) and the
-- reason token rotation was impractical.
--
-- The refresh token is a 256-bit CSPRNG value, so a SHA-256 digest is a
-- cryptographically sufficient lookup key (no brute-force surface); the
-- existing bcrypt token_hash is still verified after the row is located, for
-- defense in depth.
--
-- Existing tokens have no lookup_hash and will simply not be found on refresh
-- (those sessions re-login once) — an acceptable one-time cost that avoids
-- keeping the vulnerable scan as a fallback.
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS lookup_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_lookup_hash
    ON refresh_tokens (lookup_hash)
    WHERE lookup_hash IS NOT NULL;
