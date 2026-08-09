-- Reverse the seed by slug. Subscriptions and posts will
-- cascade-delete with the community row, so this is intentionally
-- aggressive — only safe to run on environments where the new
-- slugs haven't yet collected real content.

DELETE FROM communities WHERE slug IN (
    'paleontology', 'anthropology', 'evolution', 'pharmacology', 'aerospace',
    'ai-tools', 'self-hosting', 'linux', 'networking', 'quantum', 'cloud', '3d-printing',
    'poetry', 'languages', 'tabletop', 'podcasts', 'comedy', 'anime',
    'religion', 'drug-policy', 'demography', 'prison-reform', 'aging',
    'coffee', 'beer', 'skincare', 'fashion', 'cycling', 'running',
    'self-improvement', 'therapy', 'stoicism',
    'sales', 'real-estate', 'side-hustles'
);
