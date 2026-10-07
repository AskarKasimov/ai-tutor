-- +goose Up
CREATE TABLE variants (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    create_request_key text NOT NULL CHECK (length(create_request_key) BETWEEN 1 AND 128),
    map_revision bigint NOT NULL CHECK (map_revision >= 0),
    algorithm_version text NOT NULL,
    included_competency_count integer NOT NULL CHECK (included_competency_count > 0),
    skipped_competencies jsonb NOT NULL CHECK (jsonb_typeof(skipped_competencies) = 'array'),
    created_at bigint NOT NULL CHECK (created_at >= 0),
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

-- +goose Down
DROP TABLE variant_tasks;
DROP TABLE variants;
