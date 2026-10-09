-- +goose Up
CREATE TABLE training_sessions (
 id text PRIMARY KEY,
 owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 diagnostic_id text NOT NULL UNIQUE REFERENCES diagnostic_sessions(id) ON DELETE RESTRICT,
 subject_id text NOT NULL REFERENCES subjects(id),
 state_data jsonb NOT NULL,
 reservation_data jsonb,
 lease_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE training_start_requests (
 owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 request_key text NOT NULL,
 diagnostic_id text NOT NULL,
 session_id text NOT NULL REFERENCES training_sessions(id) ON DELETE CASCADE,
 PRIMARY KEY(owner_id,request_key)
);
CREATE TABLE training_targets (
 session_id text NOT NULL REFERENCES training_sessions(id) ON DELETE CASCADE,
 position integer NOT NULL CHECK(position>=0),
 target_data jsonb NOT NULL,
 PRIMARY KEY(session_id,position)
);
CREATE TABLE training_exercises (
 id text PRIMARY KEY,
 session_id text NOT NULL REFERENCES training_sessions(id) ON DELETE CASCADE,
 round bigint NOT NULL CHECK(round>0),
 target_index integer NOT NULL,
 exercise_data jsonb NOT NULL,
 audio_asset_id text REFERENCES audio_assets(id),
 UNIQUE(session_id,round,target_index)
);
CREATE TABLE training_attempts (
 session_id text NOT NULL REFERENCES training_sessions(id) ON DELETE CASCADE,
 sequence bigint NOT NULL CHECK(sequence>0),
 request_key text NOT NULL,
 fingerprint text NOT NULL,
 exercise_id text NOT NULL REFERENCES training_exercises(id),
 attempt_data jsonb NOT NULL,
 response_data jsonb NOT NULL,
 PRIMARY KEY(session_id,sequence),
 UNIQUE(session_id,request_key),
 UNIQUE(exercise_id)
);
-- +goose Down
DROP TABLE training_attempts;
DROP TABLE training_exercises;
DROP TABLE training_targets;
DROP TABLE training_start_requests;
DROP TABLE training_sessions;
