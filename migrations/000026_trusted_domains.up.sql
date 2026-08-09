-- Trusted domains for source validation (managed via admin, not hardcoded)
CREATE TABLE trusted_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain VARCHAR(255) NOT NULL UNIQUE,
    category VARCHAR(50) NOT NULL DEFAULT 'news',
    added_by UUID REFERENCES participants(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_trusted_domains_domain ON trusted_domains(domain);

-- Seed with common trusted sources
INSERT INTO trusted_domains (domain, category) VALUES
  ('npr.org', 'news'), ('bbc.com', 'news'), ('bbc.co.uk', 'news'),
  ('reuters.com', 'news'), ('apnews.com', 'news'),
  ('nytimes.com', 'news'), ('washingtonpost.com', 'news'),
  ('theguardian.com', 'news'), ('cnn.com', 'news'),
  ('bloomberg.com', 'finance'), ('ft.com', 'finance'), ('wsj.com', 'finance'),
  ('cnbc.com', 'finance'), ('finextra.com', 'finance'),
  ('arstechnica.com', 'tech'), ('techcrunch.com', 'tech'),
  ('theverge.com', 'tech'), ('wired.com', 'tech'),
  ('venturebeat.com', 'tech'), ('zdnet.com', 'tech'),
  ('arxiv.org', 'research'), ('nature.com', 'research'),
  ('science.org', 'research'), ('ieee.org', 'research'),
  ('nist.gov', 'government'), ('nih.gov', 'government'),
  ('sec.gov', 'government'), ('fda.gov', 'government'),
  ('github.com', 'tech'), ('openai.com', 'tech'),
  ('anthropic.com', 'tech'), ('huggingface.co', 'tech'),
  ('en.wikipedia.org', 'reference'), ('wikipedia.org', 'reference'),
  ('news.ycombinator.com', 'tech')
ON CONFLICT (domain) DO NOTHING;
