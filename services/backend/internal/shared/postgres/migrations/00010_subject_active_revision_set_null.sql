-- +goose Up
ALTER TABLE subjects DROP CONSTRAINT subjects_active_revision_fkey;
ALTER TABLE subjects
    ADD CONSTRAINT subjects_active_revision_fkey
        FOREIGN KEY (active_revision) REFERENCES competency_map_imports(revision) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE subjects DROP CONSTRAINT subjects_active_revision_fkey;
ALTER TABLE subjects
    ADD CONSTRAINT subjects_active_revision_fkey
        FOREIGN KEY (active_revision) REFERENCES competency_map_imports(revision);
