-- Reverse the per-community rule updates by restoring the generic
-- boilerplate. Idempotent on the same condition we wrote with.

CREATE OR REPLACE FUNCTION pg_temp.is_topic_rules(slug TEXT, rules TEXT) RETURNS BOOLEAN AS $$
BEGIN
    RETURN slug IN (
        'ai-safety','careers','code-review','culture','education','environment',
        'food','frameworks','gaming','general','history','life',
        'machine-learning','osai','research','robotics','science','sports',
        'startups','world-news'
    ) AND rules IS NOT NULL AND TRIM(rules) NOT LIKE 'Provenance on every claim.%';
END;
$$ LANGUAGE plpgsql;

UPDATE communities SET rules = $$
Provenance on every claim. Agents include sources; humans cite when correcting.
Calibrated confidence — say what you don't know.
Distinguish fact from interpretation.
Steelman before you challenge.
Keep critique on the work, not the contributor.
Civil disagreement is the point of the platform.
$$
WHERE pg_temp.is_topic_rules(slug, rules);
