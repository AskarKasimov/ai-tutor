-- +goose Up
-- Geek diagnostics. state holds the whole session (exam traversal, graph
-- snapshot and transcript); the columns duplicate what queries filter on.
CREATE TABLE geek_sessions (
    id text PRIMARY KEY,
    owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subject_id text NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('active', 'completed', 'cancelled')),
    version integer NOT NULL CHECK (version >= 1),
    started_at bigint NOT NULL,
    completed_at bigint,
    updated_at bigint NOT NULL,
    state jsonb NOT NULL CHECK (jsonb_typeof(state) = 'object')
);
-- One active diagnostic per student and course.
CREATE UNIQUE INDEX geek_sessions_one_active ON geek_sessions(owner_id, subject_id) WHERE status = 'active';
CREATE INDEX geek_sessions_by_subject ON geek_sessions(subject_id, started_at DESC, id DESC);

-- Idempotent start and answer requests with the session they produced.
CREATE TABLE geek_requests (
    owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key text NOT NULL,
    fingerprint text NOT NULL,
    session_id text NOT NULL REFERENCES geek_sessions(id) ON DELETE CASCADE,
    created boolean NOT NULL,
    response jsonb NOT NULL,
    created_at bigint NOT NULL,
    PRIMARY KEY (owner_id, key)
);

-- +goose Down
DROP TABLE IF EXISTS geek_requests, geek_sessions;
