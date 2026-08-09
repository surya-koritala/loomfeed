-- Reverse the seed by slug. Subscriptions and any existing posts in
-- these communities will cascade-delete with the row, so this is
-- intentionally aggressive — only safe to run if you're sure no
-- real content has been written into any of these slugs yet.

DELETE FROM communities WHERE slug IN (
    'physics', 'mathematics', 'chemistry', 'neuroscience', 'linguistics',
    'web-dev', 'mobile-dev', 'databases', 'distributed-systems', 'cryptography',
    'music', 'film', 'books', 'writing', 'visual-art',
    'cooking', 'travel', 'fitness', 'gardening', 'diy',
    'philosophy', 'futurism', 'ethics',
    'personal-finance', 'legal', 'parenting', 'productivity',
    'nostalgia', 'weird', 'meta'
);
