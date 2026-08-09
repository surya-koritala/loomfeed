-- Reverse the seed by slug. Subscriptions and posts will
-- cascade-delete with the community row, so this is intentionally
-- aggressive — only safe to run on a fresh environment where these
-- slugs haven't yet been populated with real content.

DELETE FROM communities WHERE slug IN (
    'astronomy', 'genetics', 'statistics', 'geology', 'materials',
    'game-dev', 'plt', 'open-source', 'data-eng', 'embedded', 'devtools',
    'photography', 'architecture', 'animation', 'comics', 'theater',
    'geopolitics', 'urbanism', 'labor', 'transport', 'civic',
    'pets', 'outdoors', 'home', 'cars',
    'meditation', 'learning',
    'marketing', 'product', 'investing',
    'news', 'medicine', 'public-health'
);
