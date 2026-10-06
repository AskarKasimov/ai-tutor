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

CREATE TABLE competency_map_state (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    revision bigint NOT NULL DEFAULT 0
);
INSERT INTO competency_map_state(singleton, revision) VALUES (true, 0);

CREATE TABLE competency_map_imports (
    revision bigint PRIMARY KEY,
    singleton boolean NOT NULL DEFAULT true CHECK (singleton),
    imported_at bigint NOT NULL,
    imported_by text NOT NULL REFERENCES users(id),
    competency_count integer NOT NULL CHECK (competency_count >= 0),
    constituent_count integer NOT NULL CHECK (constituent_count >= 0),
    outcome_count integer NOT NULL CHECK (outcome_count >= 0),
    task_count integer NOT NULL CHECK (task_count >= 0),
    source_format text NOT NULL CHECK (source_format IN ('paired', 'ml-map')),
    source_headers jsonb NOT NULL CHECK (jsonb_typeof(source_headers) = 'array'),
    unparsed_task_cell_count integer NOT NULL DEFAULT 0 CHECK (unparsed_task_cell_count >= 0),
    UNIQUE(singleton)
);

CREATE TABLE competency_map_source_rows (
    revision bigint NOT NULL REFERENCES competency_map_imports(revision) ON DELETE CASCADE,
    row_index integer NOT NULL CHECK (row_index > 0),
    source_line integer NOT NULL CHECK (source_line > 0),
    cells jsonb NOT NULL CHECK (jsonb_typeof(cells) = 'array'),
    PRIMARY KEY (revision, row_index)
);

CREATE TABLE competencies (
    id text PRIMARY KEY,
    name text NOT NULL,
    revision bigint NOT NULL REFERENCES competency_map_imports(revision) ON DELETE CASCADE,
    UNIQUE(id, revision)
);
CREATE INDEX competencies_by_revision ON competencies(revision);

CREATE TABLE constituents (
    id text PRIMARY KEY,
    competency_id text NOT NULL REFERENCES competencies(id) ON DELETE CASCADE,
    name text NOT NULL,
    UNIQUE(competency_id, name)
);
CREATE INDEX constituents_by_competency ON constituents(competency_id);

CREATE TABLE taxonomies (
    id text PRIMARY KEY,
    code text NOT NULL UNIQUE,
    label text NOT NULL
);
INSERT INTO taxonomies(id, code, label) VALUES
    ('taxonomy:knowledge', 'knowledge', 'Знание'),
    ('taxonomy:understanding', 'understanding', 'Понимание'),
    ('taxonomy:application', 'application', 'Применение'),
    ('taxonomy:analysis', 'analysis', 'Анализ');

CREATE TABLE ald_levels (
    id text PRIMARY KEY,
    code text NOT NULL UNIQUE,
    label text NOT NULL
);
INSERT INTO ald_levels(id, code, label) VALUES
    ('ald:basic', 'basic', 'Базовый'),
    ('ald:intermediate', 'intermediate', 'Средний'),
    ('ald:advanced', 'advanced', 'Продвинутый');

CREATE TABLE topic_levels (
    id text PRIMARY KEY,
    code text NOT NULL UNIQUE,
    label text NOT NULL
);
INSERT INTO topic_levels(id, code, label) VALUES
    ('topic:basic', 'basic', 'Базовый'),
    ('topic:intermediate', 'intermediate', 'Средний'),
    ('topic:advanced', 'advanced', 'Продвинутый');

ALTER TABLE constituents ADD COLUMN topic_level_id text REFERENCES topic_levels(id);

CREATE TABLE outcomes (
    id text PRIMARY KEY,
    constituent_id text NOT NULL REFERENCES constituents(id) ON DELETE CASCADE,
    name text NOT NULL,
    include_in_test boolean,
    taxonomy_id text REFERENCES taxonomies(id),
    ald_level_id text REFERENCES ald_levels(id),
    importance smallint CHECK (importance BETWEEN 1 AND 5),
    educational_content text,
    UNIQUE(constituent_id, name)
    -- ML completeness is checked by its parser; paired profiles may be partial.
);
CREATE INDEX outcomes_by_constituent ON outcomes(constituent_id);
CREATE INDEX outcomes_by_profile ON outcomes(include_in_test, taxonomy_id, ald_level_id, importance);

CREATE TABLE curriculum_sections (
    id text PRIMARY KEY,
    revision bigint NOT NULL REFERENCES competency_map_imports(revision) ON DELETE CASCADE,
    code text NOT NULL,
    title text NOT NULL,
    UNIQUE(revision, code),
    UNIQUE(id, revision)
);

CREATE TABLE curriculum_competencies (
    id text PRIMARY KEY,
    revision bigint NOT NULL REFERENCES competency_map_imports(revision) ON DELETE CASCADE,
    code text NOT NULL,
    UNIQUE(revision, code),
    UNIQUE(id, revision)
);

CREATE TABLE constituent_sections (
    id text PRIMARY KEY,
    revision bigint NOT NULL REFERENCES competency_map_imports(revision) ON DELETE CASCADE,
    constituent_id text NOT NULL REFERENCES constituents(id) ON DELETE CASCADE,
    section_id text NOT NULL,
    UNIQUE(revision, constituent_id, section_id),
    UNIQUE(id, revision),
    FOREIGN KEY(section_id, revision) REFERENCES curriculum_sections(id, revision) ON DELETE CASCADE
);
CREATE INDEX constituent_sections_by_section ON constituent_sections(section_id, constituent_id);

CREATE TABLE constituent_section_competencies (
    revision bigint NOT NULL REFERENCES competency_map_imports(revision) ON DELETE CASCADE,
    constituent_section_id text NOT NULL,
    curriculum_competency_id text NOT NULL,
    PRIMARY KEY(constituent_section_id, curriculum_competency_id),
    FOREIGN KEY(constituent_section_id, revision) REFERENCES constituent_sections(id, revision) ON DELETE CASCADE,
    FOREIGN KEY(curriculum_competency_id, revision) REFERENCES curriculum_competencies(id, revision) ON DELETE CASCADE
);
CREATE INDEX constituent_section_competencies_by_code ON constituent_section_competencies(curriculum_competency_id, constituent_section_id);

CREATE TABLE generation_runs (
    id text PRIMARY KEY,
    outcome_id text NOT NULL REFERENCES outcomes(id) ON DELETE CASCADE,
    map_revision bigint NOT NULL REFERENCES competency_map_imports(revision) ON DELETE CASCADE,
    model text NOT NULL,
    prompt_version text NOT NULL,
    request_key text NOT NULL,
    requested_profile jsonb NOT NULL CHECK (jsonb_typeof(requested_profile) = 'object'),
    created_at bigint NOT NULL,
    UNIQUE(id, outcome_id),
    UNIQUE(outcome_id, request_key)
);

CREATE TABLE tasks (
    id text PRIMARY KEY,
    outcome_id text NOT NULL REFERENCES outcomes(id) ON DELETE CASCADE,
    question text NOT NULL CHECK (length(btrim(question)) > 0),
    criteria text,
    source_row integer,
    source_column text,
    options jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(options) = 'array'),
    voice_instruction text,
    reference_answer text,
    origin text NOT NULL DEFAULT 'authored' CHECK (origin IN ('authored', 'ai_generated')),
    generation_run_id text,
    source_revision bigint,
    source_row_index integer,
    source_column_index integer,
    created_at bigint NOT NULL,
    CHECK ((source_row IS NULL) = (source_column IS NULL)),
    CHECK ((source_revision IS NULL AND source_row_index IS NULL AND source_column_index IS NULL) OR
        (source_revision IS NOT NULL AND source_row_index IS NOT NULL AND source_row_index > 0
         AND source_column_index IS NOT NULL AND source_column_index > 0)),
    CHECK ((origin = 'authored' AND generation_run_id IS NULL) OR
        (origin = 'ai_generated' AND generation_run_id IS NOT NULL AND source_row IS NULL AND source_column IS NULL
         AND NULLIF(btrim(voice_instruction), '') IS NOT NULL)),
    FOREIGN KEY(generation_run_id, outcome_id) REFERENCES generation_runs(id, outcome_id) ON DELETE CASCADE,
    FOREIGN KEY(source_revision, source_row_index) REFERENCES competency_map_source_rows(revision, row_index) ON DELETE CASCADE
);
CREATE INDEX tasks_by_outcome ON tasks(outcome_id, created_at, id);
CREATE INDEX tasks_origin_outcome ON tasks(outcome_id, origin);

CREATE TABLE outcome_source_rows (
    outcome_id text NOT NULL REFERENCES outcomes(id) ON DELETE CASCADE,
    source_revision bigint NOT NULL,
    source_row_index integer NOT NULL,
    PRIMARY KEY(outcome_id, source_revision, source_row_index),
    UNIQUE(source_revision, source_row_index),
    FOREIGN KEY(source_revision, source_row_index)
        REFERENCES competency_map_source_rows(revision, row_index) ON DELETE CASCADE
);

CREATE TABLE material_chunks (
    id text PRIMARY KEY,
    material_name text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    content text NOT NULL CHECK (length(btrim(content)) > 0),
    created_at bigint NOT NULL,
    search_vector tsvector GENERATED ALWAYS AS (to_tsvector('russian', content)) STORED
);
CREATE INDEX material_chunks_search ON material_chunks USING gin(search_vector);

CREATE TABLE material_chunk_outcomes (
    chunk_id text NOT NULL REFERENCES material_chunks(id) ON DELETE CASCADE,
    outcome_id text NOT NULL REFERENCES outcomes(id) ON DELETE CASCADE,
    PRIMARY KEY(chunk_id, outcome_id)
);
CREATE INDEX material_chunk_outcomes_by_outcome ON material_chunk_outcomes(outcome_id, chunk_id);

CREATE TABLE generation_run_examples (
    generation_run_id text NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    task_id text NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    PRIMARY KEY(generation_run_id, task_id)
);

CREATE TABLE generation_run_chunks (
    generation_run_id text NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    chunk_id text NOT NULL REFERENCES material_chunks(id) ON DELETE CASCADE,
    PRIMARY KEY(generation_run_id, chunk_id)
);

CREATE TABLE material_embeddings (
    chunk_id text NOT NULL REFERENCES material_chunks(id) ON DELETE CASCADE,
    model text NOT NULL,
    dimensions integer NOT NULL CHECK (dimensions > 0),
    map_revision bigint NOT NULL REFERENCES competency_map_imports(revision) ON DELETE CASCADE,
    embedding double precision[] NOT NULL,
    PRIMARY KEY(chunk_id, model, map_revision),
    CHECK (cardinality(embedding) = dimensions)
);

CREATE TABLE learning_sessions (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    map_revision bigint NOT NULL,
    map_snapshot jsonb NOT NULL CHECK (jsonb_typeof(map_snapshot) = 'object'),
    status text NOT NULL CHECK (status IN ('active', 'completed', 'abandoned')),
    created_at bigint NOT NULL,
    completed_at bigint
);
CREATE INDEX learning_sessions_by_user ON learning_sessions(user_id, created_at DESC);

CREATE TABLE task_assignments (
    id text PRIMARY KEY,
    session_id text NOT NULL REFERENCES learning_sessions(id) ON DELETE CASCADE,
    task_id text REFERENCES tasks(id) ON DELETE SET NULL,
    sequence integer NOT NULL CHECK (sequence > 0),
    role text NOT NULL CHECK (role IN ('main', 'basic', 'training')),
    task_snapshot jsonb NOT NULL CHECK (jsonb_typeof(task_snapshot) = 'object'),
    profile_snapshot jsonb NOT NULL CHECK (jsonb_typeof(profile_snapshot) = 'object'),
    created_at bigint NOT NULL,
    UNIQUE(session_id, sequence)
);
CREATE INDEX task_assignments_by_task ON task_assignments(task_id) WHERE task_id IS NOT NULL;

CREATE TABLE assignment_responses (
    id text PRIMARY KEY,
    assignment_id text NOT NULL REFERENCES task_assignments(id) ON DELETE CASCADE,
    transcription_id text REFERENCES transcriptions(id) ON DELETE SET NULL,
    answer_text text NOT NULL,
    score smallint CHECK (score BETWEEN 0 AND 2),
    feedback jsonb CHECK (feedback IS NULL OR
        CASE WHEN jsonb_typeof(feedback) = 'array' THEN jsonb_array_length(feedback) = 3 ELSE false END),
    created_at bigint NOT NULL
);
CREATE INDEX assignment_responses_by_assignment ON assignment_responses(assignment_id, created_at);
