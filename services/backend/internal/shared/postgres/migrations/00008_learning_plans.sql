-- +goose Up
-- Time the map plans for each outcome: theory and practice, in minutes.
-- Empty cells fall back to defaults when a learning plan is built.
ALTER TABLE outcomes
    ADD COLUMN theory_minutes smallint CHECK (theory_minutes BETWEEN 0 AND 600),
    ADD COLUMN practice_minutes smallint CHECK (practice_minutes BETWEEN 0 AND 600);

-- A personal learning plan built from a completed diagnostic: the outcomes of
-- every competency with a mistake, fitted into the hours the student has.
CREATE TABLE learning_plans (
    id text PRIMARY KEY,
    owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    diagnostic_id text NOT NULL REFERENCES diagnostic_sessions(id) ON DELETE CASCADE,
    plan_data jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, diagnostic_id)
);

-- The lesson chat: theory, practice questions, the student's answers and the
-- tutor's feedback, kept per plan item.
CREATE TABLE learning_messages (
    plan_id text NOT NULL REFERENCES learning_plans(id) ON DELETE CASCADE,
    sequence bigint NOT NULL CHECK (sequence > 0),
    item_index integer NOT NULL CHECK (item_index >= 0),
    role text NOT NULL CHECK (role IN ('assistant', 'user')),
    kind text NOT NULL CHECK (kind IN ('theory', 'practice', 'answer', 'feedback')),
    body text NOT NULL,
    created_at bigint NOT NULL,
    PRIMARY KEY (plan_id, sequence)
);
CREATE INDEX learning_messages_by_item ON learning_messages(plan_id, item_index, sequence);

-- +goose Down
DROP TABLE learning_messages;
DROP TABLE learning_plans;
ALTER TABLE outcomes DROP COLUMN practice_minutes, DROP COLUMN theory_minutes;
