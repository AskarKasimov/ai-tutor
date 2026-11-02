-- +goose Up
-- The model's overall conclusion of a completed exam: the first saved text
-- stays, so the student always reads the same one.
CREATE TABLE geek_result_summaries (
    session_id text PRIMARY KEY REFERENCES geek_sessions(id) ON DELETE CASCADE,
    owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    summary text NOT NULL,
    created_at bigint NOT NULL
);

-- +goose Down
DROP TABLE geek_result_summaries;
