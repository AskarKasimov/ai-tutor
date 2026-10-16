-- +goose Up
CREATE TABLE material_cleaning_jobs (
    material_id text PRIMARY KEY REFERENCES subject_materials(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'ready', 'failed')),
    attempts integer NOT NULL DEFAULT 0,
    available_at bigint NOT NULL DEFAULT 0,
    lease_until bigint NOT NULL DEFAULT 0,
    lease_token text NOT NULL DEFAULT '',
    cleaned_text text NOT NULL DEFAULT '',
    s3_cleaned_key text NOT NULL DEFAULT '',
    s3_report_key text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    updated_at bigint NOT NULL
);
CREATE INDEX material_cleaning_pending_idx ON material_cleaning_jobs(status, available_at, lease_until);
-- Existing materials are deliberately not queued by a schema migration.

-- +goose Down
DROP TABLE material_cleaning_jobs;
