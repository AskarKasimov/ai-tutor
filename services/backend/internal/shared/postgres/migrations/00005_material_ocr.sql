-- +goose Up
CREATE TABLE material_ocr_jobs (
    material_id text PRIMARY KEY REFERENCES subject_materials(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'ready', 'failed')),
    attempts integer NOT NULL DEFAULT 0,
    available_at bigint NOT NULL DEFAULT 0,
    lease_until bigint NOT NULL DEFAULT 0,
    lease_token text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    updated_at bigint NOT NULL
);

CREATE INDEX material_ocr_pending_idx ON material_ocr_jobs(status, available_at, lease_until);

-- +goose Down
DROP TABLE material_ocr_jobs;
