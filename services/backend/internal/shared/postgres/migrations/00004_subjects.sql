-- +goose Up
CREATE TABLE subjects (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    active_revision bigint,
    created_at bigint NOT NULL CHECK (created_at >= 0)
);

INSERT INTO subjects(id, name, active_revision, created_at)
SELECT 'subject:intro-to-ml', 'Введение в ML',
       NULLIF(state.revision, 0), 0
FROM competency_map_state state
WHERE state.singleton = true;

ALTER TABLE competency_map_imports
    DROP CONSTRAINT competency_map_imports_singleton_key;
ALTER TABLE competency_map_imports
    ADD COLUMN subject_id text NOT NULL DEFAULT 'subject:intro-to-ml'
        REFERENCES subjects(id);
ALTER TABLE competency_map_imports
    ADD CONSTRAINT competency_map_imports_one_per_subject UNIQUE(subject_id);

ALTER TABLE subjects
    ADD CONSTRAINT subjects_active_revision_fkey
        FOREIGN KEY (active_revision) REFERENCES competency_map_imports(revision);

ALTER TABLE variants
    ADD COLUMN subject_id text NOT NULL DEFAULT 'subject:intro-to-ml'
        REFERENCES subjects(id),
    ADD COLUMN subject_name_snapshot text NOT NULL DEFAULT 'Введение в ML';

-- The global counter remains monotonic and independent of per-subject activation.
-- Existing state already contains the highest allocated revision.

-- +goose Down
ALTER TABLE variants DROP COLUMN subject_name_snapshot;
ALTER TABLE variants DROP COLUMN subject_id;
ALTER TABLE subjects DROP CONSTRAINT subjects_active_revision_fkey;
ALTER TABLE competency_map_imports DROP CONSTRAINT competency_map_imports_one_per_subject;
ALTER TABLE competency_map_imports DROP COLUMN subject_id;
ALTER TABLE competency_map_imports
    ADD CONSTRAINT competency_map_imports_singleton_key UNIQUE(singleton);
DROP TABLE subjects;
