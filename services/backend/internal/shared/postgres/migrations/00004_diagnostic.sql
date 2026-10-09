-- +goose Up
CREATE TABLE diagnostic_sessions (
    id text PRIMARY KEY,
    owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    variant_id text NOT NULL REFERENCES variants(id) ON DELETE RESTRICT,
    subject_id text NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    subject_name_snapshot text NOT NULL CHECK (length(btrim(subject_name_snapshot)) > 0),
    map_revision bigint NOT NULL CHECK (map_revision >= 0),
    status text NOT NULL CHECK (status IN ('active', 'completed')),
    current_competency integer NOT NULL CHECK (current_competency >= 0),
    current_task integer NOT NULL CHECK (current_task >= 0),
    start_request_key text NOT NULL CHECK (length(start_request_key) BETWEEN 1 AND 128),
    start_request_digest text NOT NULL CHECK (length(start_request_digest) > 0),
    session_data jsonb NOT NULL CHECK (jsonb_typeof(session_data) = 'object'),
    in_flight_key text,
    in_flight_fingerprint text,
    in_flight_variant_task_id text,
    in_flight_token text,
    lease_until timestamptz,
    transcription_id text,
    transcription_text text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE(owner_id, start_request_key),
    CHECK ((in_flight_key IS NULL AND in_flight_fingerprint IS NULL AND in_flight_variant_task_id IS NULL)
        OR (in_flight_key IS NOT NULL AND in_flight_fingerprint IS NOT NULL AND in_flight_variant_task_id IS NOT NULL))
);
CREATE INDEX diagnostic_sessions_by_owner_history ON diagnostic_sessions(owner_id, completed_at DESC, id DESC);
CREATE INDEX diagnostic_sessions_by_subject_completion ON diagnostic_sessions(subject_id, completed_at DESC, id DESC)
    WHERE status = 'completed';

CREATE TABLE diagnostic_answers (
    session_id text NOT NULL REFERENCES diagnostic_sessions(id) ON DELETE CASCADE,
    answer_order integer NOT NULL CHECK (answer_order > 0),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    request_digest text NOT NULL CHECK (length(request_digest) > 0),
    variant_task_id text NOT NULL,
    answer_data jsonb NOT NULL CHECK (jsonb_typeof(answer_data) = 'object'),
    progress_data jsonb NOT NULL CHECK (jsonb_typeof(progress_data) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(session_id, idempotency_key),
    UNIQUE(session_id, answer_order)
);

-- +goose Down
DROP TABLE diagnostic_answers;
DROP TABLE diagnostic_sessions;
