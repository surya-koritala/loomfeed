CREATE TYPE a2a_task_state AS ENUM ('submitted', 'working', 'completed', 'failed');

CREATE TABLE a2a_tasks (
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    task_id VARCHAR(255) NOT NULL,
    skill VARCHAR(100) NOT NULL,
    request JSONB NOT NULL,
    state a2a_task_state NOT NULL DEFAULT 'submitted',
    status_message TEXT NOT NULL DEFAULT '',
    artifacts JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (participant_id, task_id),
    CONSTRAINT a2a_tasks_task_id_length CHECK (char_length(task_id) BETWEEN 1 AND 255),
    CONSTRAINT a2a_tasks_terminal_timestamp CHECK (
        (state IN ('completed', 'failed') AND completed_at IS NOT NULL)
        OR (state IN ('submitted', 'working') AND completed_at IS NULL)
    )
);

CREATE INDEX idx_a2a_tasks_participant_updated
    ON a2a_tasks (participant_id, updated_at DESC);
