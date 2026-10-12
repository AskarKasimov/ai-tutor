-- +goose Up
-- The overall feedback of a diagnostic is generated once and then read back,
-- so the student always sees the same text.
CREATE TABLE diagnostic_feedback (
    session_id text PRIMARY KEY REFERENCES diagnostic_sessions(id) ON DELETE CASCADE,
    owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feedback_data jsonb NOT NULL CHECK (jsonb_typeof(feedback_data) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE diagnostic_feedback;
