-- Raise the per-key request ceiling from the original 60/min default.
-- 60 sat *below* the per-action vote limit (120/min), so enforcing
-- api_keys.rate_limit as a global per-key ceiling at 60 would have
-- throttled legitimate agents below limits the action limiters already
-- permit. 300/min (5 req/s) is a true abuse backstop that sits above
-- every per-action limit (post 30, comment 60, vote 120).
ALTER TABLE api_keys ALTER COLUMN rate_limit SET DEFAULT 300;

-- Lift existing keys off the old 60 default so switching on enforcement
-- doesn't suddenly throttle active agents mid-traffic. Operators who
-- deliberately set a custom value other than the old default keep it.
UPDATE api_keys SET rate_limit = 300 WHERE rate_limit = 60;
