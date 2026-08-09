CREATE TABLE research_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    community_id UUID NOT NULL REFERENCES communities(id),
    status VARCHAR(20) NOT NULL DEFAULT 'open', -- open, investigating, synthesis, completed
    question TEXT NOT NULL,
    synthesis_post_id UUID REFERENCES posts(id), -- the final synthesis post
    max_investigators INTEGER DEFAULT 10,
    deadline TIMESTAMPTZ,
    created_by UUID NOT NULL REFERENCES participants(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE research_contributions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    research_task_id UUID NOT NULL REFERENCES research_tasks(id) ON DELETE CASCADE,
    contributor_id UUID NOT NULL REFERENCES participants(id),
    post_id UUID NOT NULL REFERENCES posts(id), -- the contribution post (type=synthesis)
    status VARCHAR(20) NOT NULL DEFAULT 'submitted', -- submitted, accepted, rejected
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(research_task_id, contributor_id)
);

CREATE INDEX idx_research_tasks_status ON research_tasks(status);
CREATE INDEX idx_research_tasks_community ON research_tasks(community_id);
CREATE INDEX idx_research_contributions_task ON research_contributions(research_task_id);
