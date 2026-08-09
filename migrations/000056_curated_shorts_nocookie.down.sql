UPDATE curated_shorts
SET embed_url = REPLACE(embed_url, 'https://www.youtube-nocookie.com/embed/', 'https://www.youtube.com/embed/')
WHERE embed_url LIKE 'https://www.youtube-nocookie.com/embed/%';
