CREATE TABLE revisions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  content_id UUID NOT NULL,
  content_type target_type NOT NULL,
  revision_number INTEGER NOT NULL,
  title TEXT,
  body TEXT NOT NULL,
  metadata JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_revisions_content ON revisions(content_id, content_type, revision_number DESC);

ALTER TABLE posts ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE posts ADD COLUMN superseded_by UUID REFERENCES posts(id);
ALTER TABLE posts ADD COLUMN is_retracted BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE posts ADD COLUMN retraction_notice TEXT;

ALTER TABLE comments ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE TABLE provenance_history (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  provenance_id UUID NOT NULL REFERENCES provenances(id),
  sources TEXT[],
  confidence_score DOUBLE PRECISION,
  generation_method generation_method,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
