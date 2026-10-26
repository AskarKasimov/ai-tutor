-- +goose Up
ALTER TABLE subjects ADD COLUMN mode text NOT NULL DEFAULT 'exam' CHECK (mode IN ('exam', 'learn'));
ALTER TABLE subjects ADD COLUMN active_revision bigint;
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
    subject_id text NOT NULL REFERENCES subjects(id),
    UNIQUE(subject_id)
);
ALTER TABLE subjects ADD CONSTRAINT subjects_active_revision_fkey
    FOREIGN KEY (active_revision) REFERENCES competency_map_imports(revision) ON DELETE SET NULL;

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
    subject_id text NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    material_name text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    content text NOT NULL CHECK (length(btrim(content)) > 0),
    created_at bigint NOT NULL,
    search_vector tsvector GENERATED ALWAYS AS (to_tsvector('russian', content)) STORED
);
CREATE INDEX material_chunks_search ON material_chunks USING gin(search_vector);
CREATE INDEX material_chunks_subject_name ON material_chunks(subject_id, material_name);

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

CREATE VIEW constituent_curriculum_profiles AS
SELECT constituent.id AS constituent_id,
    COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'code', section.code,
            'title', section.title,
            'curriculum_competencies', COALESCE((
                SELECT jsonb_agg(curriculum.code ORDER BY curriculum.code)
                FROM constituent_section_competencies AS mapped
                JOIN curriculum_competencies AS curriculum ON curriculum.id = mapped.curriculum_competency_id
                WHERE mapped.constituent_section_id = link.id
            ), '[]'::jsonb)
        ) ORDER BY section.code)
        FROM constituent_sections AS link
        JOIN curriculum_sections AS section ON section.id = link.section_id
        WHERE link.constituent_id = constituent.id
    ), '[]'::jsonb)::text AS curriculum_sections
FROM constituents AS constituent;


-- Imported from legacy migration 00002_variants.sql

CREATE TABLE variants (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    create_request_key text NOT NULL CHECK (length(create_request_key) BETWEEN 1 AND 128),
    map_revision bigint NOT NULL CHECK (map_revision >= 0),
    algorithm_version text NOT NULL,
    included_competency_count integer NOT NULL CHECK (included_competency_count > 0),
    skipped_competencies jsonb NOT NULL CHECK (jsonb_typeof(skipped_competencies) = 'array'),
    created_at bigint NOT NULL CHECK (created_at >= 0),
    subject_id text NOT NULL REFERENCES subjects(id),
    subject_name_snapshot text NOT NULL,
    UNIQUE(user_id, create_request_key)
);
CREATE INDEX variants_by_owner_history ON variants(user_id, created_at DESC, id DESC);

CREATE TABLE variant_tasks (
    id text PRIMARY KEY,
    variant_id text NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
    live_task_id text REFERENCES tasks(id) ON DELETE SET NULL,
    source_task_id_snapshot text NOT NULL,
    competency_position integer NOT NULL CHECK (competency_position > 0),
    slot smallint NOT NULL CHECK (slot BETWEEN 0 AND 2),
    role text NOT NULL CHECK ((slot = 0 AND role = 'main') OR (slot IN (1, 2) AND role = 'basic')),
    task_snapshot jsonb NOT NULL CHECK (jsonb_typeof(task_snapshot) = 'object'),
    profile_snapshot jsonb NOT NULL CHECK (jsonb_typeof(profile_snapshot) = 'object'),
    UNIQUE(variant_id, competency_position, slot),
    UNIQUE(variant_id, source_task_id_snapshot)
);
CREATE INDEX variant_tasks_by_variant ON variant_tasks(variant_id, competency_position, slot);


-- Imported from legacy migration 00003_task_audio.sql

CREATE TABLE audio_assets (
    id text PRIMARY KEY,
    instruction text NOT NULL CHECK (char_length(instruction) BETWEEN 1 AND 500 AND instruction ~ '[^[:space:]]'),
    object_key text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'ready', 'failed', 'cancelled')),
    bucket text,
    storage_uri text,
    audio_url text,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_until timestamptz,
    claim_token text,
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'ready' AND bucket IS NOT NULL AND storage_uri IS NOT NULL AND audio_url IS NOT NULL)
        OR (status <> 'ready' AND bucket IS NULL AND storage_uri IS NULL AND audio_url IS NULL)),
    CHECK ((status = 'processing' AND lease_until IS NOT NULL AND claim_token IS NOT NULL)
        OR (status <> 'processing' AND lease_until IS NULL AND claim_token IS NULL))
);
CREATE INDEX audio_assets_pending_queue ON audio_assets(next_attempt_at, created_at, id)
    WHERE status = 'pending';
CREATE INDEX audio_assets_expired_leases ON audio_assets(lease_until)
    WHERE status = 'processing';

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT source_id
        FROM (
            SELECT id AS source_id, voice_instruction AS instruction
            FROM tasks
            WHERE voice_instruction IS NOT NULL
            UNION ALL
            SELECT source_task_id_snapshot AS source_id,
                   task_snapshot->>'voice_instruction' AS instruction
            FROM variant_tasks
            WHERE task_snapshot->>'voice_instruction' IS NOT NULL
        ) instructions
        GROUP BY source_id
        HAVING count(DISTINCT instruction) > 1
    ) THEN
        RAISE EXCEPTION 'task audio migration found conflicting instructions for a source task';
    END IF;
END $$;
-- +goose StatementEnd

INSERT INTO audio_assets(id, instruction, object_key)
SELECT 'taskaudio_' || task.id, task.voice_instruction,
       'task-audio/v1/taskaudio_' || task.id || '.wav'
FROM tasks task
WHERE task.voice_instruction IS NOT NULL
  AND char_length(task.voice_instruction) BETWEEN 1 AND 500
  AND task.voice_instruction ~ '[^[:space:]]'
ON CONFLICT (id) DO NOTHING;

INSERT INTO audio_assets(id, instruction, object_key)
SELECT DISTINCT ON (variant_task.source_task_id_snapshot)
       'taskaudio_' || variant_task.source_task_id_snapshot,
       variant_task.task_snapshot->>'voice_instruction',
       'task-audio/v1/taskaudio_' || variant_task.source_task_id_snapshot || '.wav'
FROM variant_tasks variant_task
WHERE variant_task.task_snapshot->>'voice_instruction' IS NOT NULL
  AND char_length(variant_task.task_snapshot->>'voice_instruction') BETWEEN 1 AND 500
  AND variant_task.task_snapshot->>'voice_instruction' ~ '[^[:space:]]'
ORDER BY variant_task.source_task_id_snapshot
ON CONFLICT (id) DO NOTHING;

ALTER TABLE tasks ADD COLUMN audio_asset_id text REFERENCES audio_assets(id) ON DELETE SET NULL;
ALTER TABLE variant_tasks ADD COLUMN audio_asset_id text REFERENCES audio_assets(id) ON DELETE SET NULL;
UPDATE tasks SET audio_asset_id = 'taskaudio_' || id
WHERE EXISTS (SELECT 1 FROM audio_assets asset WHERE asset.id = 'taskaudio_' || tasks.id);
UPDATE variant_tasks SET audio_asset_id = 'taskaudio_' || source_task_id_snapshot
WHERE EXISTS (SELECT 1 FROM audio_assets asset WHERE asset.id = 'taskaudio_' || variant_tasks.source_task_id_snapshot);
CREATE INDEX tasks_audio_asset ON tasks(audio_asset_id) WHERE audio_asset_id IS NOT NULL;
CREATE INDEX variant_tasks_audio_asset ON variant_tasks(audio_asset_id) WHERE audio_asset_id IS NOT NULL;

-- Snapshot-only work outside the active map must not consume worker capacity.
UPDATE audio_assets asset
SET status = 'cancelled', last_error_code = 'task_no_longer_current', updated_at = now()
WHERE asset.status = 'pending'
  AND NOT EXISTS (
      SELECT 1
      FROM tasks task
      JOIN outcomes outcome ON outcome.id = task.outcome_id
      JOIN constituents constituent ON constituent.id = outcome.constituent_id
      JOIN competencies competency ON competency.id = constituent.competency_id
      JOIN competency_map_state state ON state.singleton = true AND state.revision = competency.revision
      WHERE task.audio_asset_id = asset.id
  );


-- Imported from legacy migration 00004_diagnostic.sql

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


-- Imported from legacy migration 00005_training.sql

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

-- Imported from legacy migration 00006_audio_priority.sql

ALTER TABLE audio_assets ADD COLUMN priority_requested_at timestamptz;


-- Imported from legacy migration 00007_training_chat.sql

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


-- Imported from legacy migration 00008_learning_plans.sql

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


-- Imported from legacy migration 00009_diagnostic_feedback.sql

-- The overall feedback of a diagnostic is generated once and then read back,
-- so the student always sees the same text.
CREATE TABLE diagnostic_feedback (
    session_id text PRIMARY KEY REFERENCES diagnostic_sessions(id) ON DELETE CASCADE,
    owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feedback_data jsonb NOT NULL CHECK (jsonb_typeof(feedback_data) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);


-- +goose Down
DROP TABLE IF EXISTS diagnostic_feedback, learning_messages, learning_plans,
    training_chat_turns, training_chat_messages, training_chats,
    training_attempts, training_exercises, training_targets, training_start_requests,
    training_sessions, diagnostic_answers, diagnostic_sessions,
    assignment_responses, task_assignments, learning_sessions, variant_tasks, variants,
    audio_assets, material_embeddings, generation_run_chunks, generation_run_examples,
    material_chunk_outcomes, material_chunks, outcome_source_rows, tasks,
    generation_runs, constituent_section_competencies, constituent_sections,
    curriculum_competencies, curriculum_sections, outcomes, constituents,
    competencies, competency_map_source_rows, competency_map_imports,
    competency_map_state, topic_levels, ald_levels, taxonomies CASCADE;
ALTER TABLE subjects DROP COLUMN IF EXISTS active_revision;
ALTER TABLE subjects DROP COLUMN IF EXISTS mode;
