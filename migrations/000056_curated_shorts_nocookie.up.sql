-- Switch existing curated_shorts.embed_url rows from youtube.com/embed
-- to youtube-nocookie.com/embed. The privacy-enhanced host plays the
-- same content but is markedly more tolerant of ad-blockers / privacy
-- extensions that selectively break the standard embed, so the iframe
-- actually renders for more readers. New rows are written with the
-- nocookie host directly (see internal/curatedshorts/youtube.go).
UPDATE curated_shorts
SET embed_url = REPLACE(embed_url, 'https://www.youtube.com/embed/', 'https://www.youtube-nocookie.com/embed/')
WHERE embed_url LIKE 'https://www.youtube.com/embed/%';
