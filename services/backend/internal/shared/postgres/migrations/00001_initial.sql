-- +goose Up
CREATE TABLE users (
    id text PRIMARY KEY,
    email text NOT NULL UNIQUE CHECK (email = lower(btrim(email))),
    display_name text,
    password_hash text NOT NULL,
    role text NOT NULL DEFAULT 'student' CHECK (role IN ('student', 'admin')),
    created_at bigint NOT NULL CHECK (created_at >= 0)
);

CREATE TABLE auth_sessions (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at bigint NOT NULL,
    revoked_at bigint
);
CREATE INDEX auth_sessions_user ON auth_sessions(user_id);

CREATE TABLE access_tokens (
    token_hash bytea PRIMARY KEY,
    session_id text NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
    expires_at bigint NOT NULL
);
CREATE INDEX access_tokens_session ON access_tokens(session_id);

CREATE TABLE refresh_tokens (
    token_hash bytea PRIMARY KEY,
    session_id text NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
    used_at bigint
);
CREATE INDEX refresh_tokens_session ON refresh_tokens(session_id);

CREATE TABLE transcriptions (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    text text NOT NULL CHECK (length(btrim(text)) > 0),
    created_at bigint NOT NULL
);
CREATE INDEX transcriptions_owner ON transcriptions(user_id);

CREATE TABLE subjects (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    created_at bigint NOT NULL CHECK (created_at >= 0)
);

CREATE TABLE subject_materials (
    id text PRIMARY KEY,
    subject_id text NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    filename text NOT NULL,
    content_type text NOT NULL,
    file_size bigint NOT NULL DEFAULT 0,
    s3_raw_key text NOT NULL,
    s3_parsed_key text NOT NULL,
    parsed_text text NOT NULL DEFAULT '',
    char_count integer NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'parsed' CHECK (status IN ('pending', 'parsed', 'failed')),
    error_message text,
    created_at bigint NOT NULL,
    updated_at bigint NOT NULL
);
CREATE INDEX subject_materials_subject_idx ON subject_materials(subject_id);

-- +goose Down
DROP TABLE IF EXISTS subject_materials, subjects, transcriptions, refresh_tokens,
    access_tokens, auth_sessions, users CASCADE;
