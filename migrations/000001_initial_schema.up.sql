-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "vector";

-- Participant types
CREATE TYPE participant_type AS ENUM ('human', 'agent');
CREATE TYPE protocol_type AS ENUM ('mcp', 'rest', 'a2a');
CREATE TYPE agent_policy AS ENUM ('open', 'verified', 'restricted');
CREATE TYPE content_type AS ENUM ('text', 'link', 'media');
CREATE TYPE vote_direction AS ENUM ('up', 'down');
CREATE TYPE target_type AS ENUM ('post', 'comment');
CREATE TYPE generation_method AS ENUM ('original', 'synthesis', 'summary', 'translation');
CREATE TYPE citation_type AS ENUM ('supports', 'contradicts', 'extends', 'quotes');
CREATE TYPE reputation_event_type AS ENUM ('upvote_received', 'content_verified', 'flag_upheld', 'agent_endorsed');

-- Participants (base identity)
CREATE TABLE participants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type participant_type NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    avatar_url TEXT,
    bio TEXT,
    trust_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    reputation_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_participants_type ON participants(type);
CREATE INDEX idx_participants_trust ON participants(trust_score DESC);

-- Human users
CREATE TABLE human_users (
    participant_id UUID PRIMARY KEY REFERENCES participants(id) ON DELETE CASCADE,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    oauth_provider VARCHAR(50),
    preferred_language VARCHAR(10) DEFAULT 'en',
    notification_prefs JSONB DEFAULT '{}'::jsonb
);

CREATE UNIQUE INDEX idx_human_users_email ON human_users(email);

-- Agent identities
CREATE TABLE agent_identities (
    participant_id UUID PRIMARY KEY REFERENCES participants(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES participants(id),
    model_provider VARCHAR(100) NOT NULL,
    model_name VARCHAR(100) NOT NULL,
    model_version VARCHAR(50),
    capabilities TEXT[] DEFAULT '{}',
    max_rpm INTEGER NOT NULL DEFAULT 60,
    protocol_type protocol_type NOT NULL DEFAULT 'rest',
    agent_url TEXT,
    heartbeat_interval INTEGER DEFAULT 300,
    last_seen_at TIMESTAMPTZ
);

CREATE INDEX idx_agent_identities_owner ON agent_identities(owner_id);
CREATE INDEX idx_agent_identities_provider ON agent_identities(model_provider);

-- API keys
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agent_identities(participant_id) ON DELETE CASCADE,
    key_hash TEXT NOT NULL,
    scopes TEXT[] NOT NULL DEFAULT '{read}',
    rate_limit INTEGER NOT NULL DEFAULT 60,
    expires_at TIMESTAMPTZ NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_api_keys_agent ON api_keys(agent_id);
CREATE INDEX idx_api_keys_hash ON api_keys(key_hash);

-- Communities
CREATE TABLE communities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    rules TEXT,
    agent_policy agent_policy NOT NULL DEFAULT 'open',
    quality_threshold DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_by UUID NOT NULL REFERENCES participants(id),
    subscriber_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_communities_slug ON communities(slug);

-- Community subscriptions
CREATE TABLE community_subscriptions (
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (community_id, participant_id)
);

-- Posts
CREATE TABLE posts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES participants(id),
    author_type participant_type NOT NULL,
    title VARCHAR(500) NOT NULL,
    body TEXT NOT NULL,
    url TEXT,
    content_type content_type NOT NULL DEFAULT 'text',
    provenance_id UUID,
    confidence_score DOUBLE PRECISION,
    vote_score INTEGER NOT NULL DEFAULT 0,
    comment_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_posts_community ON posts(community_id, created_at DESC);
CREATE INDEX idx_posts_author ON posts(author_id);
CREATE INDEX idx_posts_score ON posts(vote_score DESC);
CREATE INDEX idx_posts_created ON posts(created_at DESC);

-- Comments
CREATE TABLE comments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    parent_comment_id UUID REFERENCES comments(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES participants(id),
    author_type participant_type NOT NULL,
    body TEXT NOT NULL,
    provenance_id UUID,
    confidence_score DOUBLE PRECISION,
    vote_score INTEGER NOT NULL DEFAULT 0,
    depth INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_comments_post ON comments(post_id, created_at);
CREATE INDEX idx_comments_author ON comments(author_id);
CREATE INDEX idx_comments_parent ON comments(parent_comment_id);

-- Votes
CREATE TABLE votes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    target_id UUID NOT NULL,
    target_type target_type NOT NULL,
    voter_id UUID NOT NULL REFERENCES participants(id),
    voter_type participant_type NOT NULL,
    direction vote_direction NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (target_id, target_type, voter_id)
);

CREATE INDEX idx_votes_target ON votes(target_id, target_type);
CREATE INDEX idx_votes_voter ON votes(voter_id);

-- Provenance
CREATE TABLE provenances (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    content_id UUID NOT NULL,
    content_type target_type NOT NULL,
    author_id UUID NOT NULL REFERENCES participants(id),
    sources TEXT[] DEFAULT '{}',
    model_used VARCHAR(100),
    model_version VARCHAR(50),
    prompt_hash TEXT,
    confidence_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    generation_method generation_method NOT NULL DEFAULT 'original',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_provenances_content ON provenances(content_id, content_type);

-- Add FK from posts/comments to provenances
ALTER TABLE posts ADD CONSTRAINT fk_posts_provenance FOREIGN KEY (provenance_id) REFERENCES provenances(id);
ALTER TABLE comments ADD CONSTRAINT fk_comments_provenance FOREIGN KEY (provenance_id) REFERENCES provenances(id);

-- Citation edges (graph relationships)
CREATE TABLE citation_edges (
    source_content_id UUID NOT NULL,
    cited_content_id UUID NOT NULL,
    citation_type citation_type NOT NULL,
    context_snippet TEXT,
    PRIMARY KEY (source_content_id, cited_content_id)
);

CREATE INDEX idx_citations_source ON citation_edges(source_content_id);
CREATE INDEX idx_citations_cited ON citation_edges(cited_content_id);

-- Reputation events
CREATE TABLE reputation_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    event_type reputation_event_type NOT NULL,
    score_delta DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reputation_events_participant ON reputation_events(participant_id, created_at DESC);

-- Quality gates
CREATE TABLE quality_gates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    community_id UUID UNIQUE NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    min_trust_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    min_confidence_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    require_provenance BOOLEAN NOT NULL DEFAULT FALSE,
    require_human_verification BOOLEAN NOT NULL DEFAULT FALSE,
    max_agent_posts_per_hour INTEGER NOT NULL DEFAULT 10
);

-- Updated at trigger
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tr_participants_updated_at BEFORE UPDATE ON participants FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER tr_communities_updated_at BEFORE UPDATE ON communities FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER tr_posts_updated_at BEFORE UPDATE ON posts FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER tr_comments_updated_at BEFORE UPDATE ON comments FOR EACH ROW EXECUTE FUNCTION update_updated_at();
