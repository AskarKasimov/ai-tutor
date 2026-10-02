-- +goose Up
CREATE TABLE
    IF NOT EXISTS users (
        id text PRIMARY KEY,
        email text NOT NULL UNIQUE CHECK (email = lower(btrim (email))),
        display_name text,
        password_hash text NOT NULL,
        role text NOT NULL DEFAULT 'student' CHECK (role IN ('student', 'admin')),
        created_at bigint NOT NULL CHECK (created_at >= 0)
    );

CREATE TABLE
    IF NOT EXISTS auth_sessions (
        id text PRIMARY KEY,
        user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
        expires_at bigint NOT NULL,
        revoked_at bigint
    );

CREATE INDEX IF NOT EXISTS auth_sessions_user ON auth_sessions (user_id);

CREATE TABLE
    IF NOT EXISTS access_tokens (
        token_hash bytea PRIMARY KEY,
        session_id text NOT NULL REFERENCES auth_sessions (id) ON DELETE CASCADE,
        expires_at bigint NOT NULL
    );

CREATE INDEX IF NOT EXISTS access_tokens_session ON access_tokens (session_id);

CREATE TABLE
    IF NOT EXISTS refresh_tokens (
        token_hash bytea PRIMARY KEY,
        session_id text NOT NULL REFERENCES auth_sessions (id) ON DELETE CASCADE,
        used_at bigint
    );

CREATE INDEX IF NOT EXISTS refresh_tokens_session ON refresh_tokens (session_id);

CREATE TABLE
    IF NOT EXISTS transcriptions (
        id text PRIMARY KEY,
        user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
        text text NOT NULL CHECK (length (btrim (text)) > 0),
        created_at bigint NOT NULL
    );

CREATE INDEX IF NOT EXISTS transcriptions_owner ON transcriptions (user_id);

CREATE TABLE
    IF NOT EXISTS competency_map_state (
        singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
        revision bigint NOT NULL DEFAULT 0
    );

INSERT INTO
    competency_map_state (singleton, revision)
VALUES
    (true, 0) ON CONFLICT DO NOTHING;

CREATE TABLE
    IF NOT EXISTS competency_map_imports (
        revision bigint PRIMARY KEY,
        imported_at bigint NOT NULL,
        imported_by text NOT NULL REFERENCES users (id),
        competency_count integer NOT NULL,
        constituent_count integer NOT NULL,
        outcome_count integer NOT NULL,
        task_count integer NOT NULL
    );

CREATE TABLE
    IF NOT EXISTS competencies (
        id text PRIMARY KEY,
        name text NOT NULL UNIQUE,
        revision bigint NOT NULL REFERENCES competency_map_imports (revision)
    );

CREATE TABLE
    IF NOT EXISTS constituents (
        id text PRIMARY KEY,
        competency_id text NOT NULL REFERENCES competencies (id) ON DELETE CASCADE,
        name text NOT NULL,
        UNIQUE (competency_id, name)
    );

CREATE TABLE
    IF NOT EXISTS outcomes (
        id text PRIMARY KEY,
        constituent_id text NOT NULL REFERENCES constituents (id) ON DELETE CASCADE,
        name text NOT NULL,
        attributes jsonb NOT NULL DEFAULT '[]',
        UNIQUE (constituent_id, name)
    );

CREATE TABLE
    IF NOT EXISTS tasks (
        id text PRIMARY KEY,
        outcome_id text NOT NULL REFERENCES outcomes (id) ON DELETE CASCADE,
        question text NOT NULL CHECK (length (btrim (question)) > 0),
        criteria text NOT NULL CHECK (length (btrim (criteria)) > 0),
        source_row integer NOT NULL,
        source_column text NOT NULL
    );
