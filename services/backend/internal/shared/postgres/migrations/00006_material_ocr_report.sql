-- +goose Up
ALTER TABLE subject_materials
    ADD COLUMN s3_ocr_key text NOT NULL DEFAULT '',
    ADD COLUMN ocr_report jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE material_ocr_jobs ADD COLUMN stage text NOT NULL DEFAULT 'queued';

-- +goose Down
ALTER TABLE subject_materials
    DROP COLUMN ocr_report,
    DROP COLUMN s3_ocr_key;
ALTER TABLE material_ocr_jobs DROP COLUMN stage;
