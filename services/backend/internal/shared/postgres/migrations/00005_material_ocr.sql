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

-- Materials uploaded before this pipeline run through it again from their raw
-- file in S3: OCR, translation, cleaning (decision 17.10.2026).
INSERT INTO material_ocr_jobs(material_id, updated_at)
SELECT id, extract(epoch FROM now())::bigint FROM subject_materials;
UPDATE subject_materials SET status = 'pending', error_message = NULL;

-- +goose Down
DROP TABLE material_ocr_jobs;
UPDATE subject_materials SET status = 'parsed' WHERE status = 'pending' AND parsed_text <> '';
