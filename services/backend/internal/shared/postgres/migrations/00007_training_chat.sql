-- +goose Up
CREATE TABLE training_chats (
    exercise_id text PRIMARY KEY REFERENCES training_exercises(id) ON DELETE CASCADE,
    session_id text NOT NULL REFERENCES training_sessions(id) ON DELETE CASCADE,
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
    latest_check_data jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, exercise_id)
);

CREATE TABLE training_chat_messages (
    id text PRIMARY KEY,
    exercise_id text NOT NULL REFERENCES training_chats(exercise_id) ON DELETE CASCADE,
    sequence bigint NOT NULL CHECK (sequence > 0),
    role text NOT NULL CHECK (role IN ('user', 'assistant')),
    kind text NOT NULL CHECK (kind IN ('welcome', 'message', 'check')),
    source text CHECK (source IS NULL OR source IN ('text', 'voice')),
    text text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (exercise_id, sequence)
);
CREATE INDEX training_chat_messages_page_idx ON training_chat_messages (exercise_id, sequence DESC);

CREATE TABLE training_chat_turns (
    id text PRIMARY KEY,
    session_id text NOT NULL REFERENCES training_sessions(id) ON DELETE CASCADE,
    exercise_id text NOT NULL,
    request_key text NOT NULL,
    operation text NOT NULL CHECK (operation IN ('message', 'check')),
    fingerprint text NOT NULL,
    expected_revision bigint NOT NULL CHECK (expected_revision >= 0),
    token text NOT NULL,
    lease_until timestamptz,
    status text NOT NULL CHECK (status IN ('processing', 'failed', 'completed', 'cancelled')),
    request_data jsonb NOT NULL,
    pending_audio bytea,
    transcription_id text,
    transcription_text text,
    check_data jsonb,
    response_data jsonb,
    error_code text,
    prompt_version text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, request_key),
    FOREIGN KEY (session_id, exercise_id) REFERENCES training_chats(session_id, exercise_id) ON DELETE CASCADE
);
CREATE INDEX training_chat_turns_pending_idx ON training_chat_turns (session_id, status, lease_until);
CREATE INDEX training_chat_turns_exercise_idx ON training_chat_turns (exercise_id, created_at);

-- +goose Down
DROP TABLE training_chat_turns;
DROP TABLE training_chat_messages;
DROP TABLE training_chats;
