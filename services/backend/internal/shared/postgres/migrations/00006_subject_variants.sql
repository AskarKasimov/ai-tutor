-- +goose Up
ALTER TABLE variants ALTER COLUMN subject_id DROP DEFAULT;
ALTER TABLE variants ALTER COLUMN subject_name_snapshot DROP DEFAULT;

-- +goose Down
ALTER TABLE variants ALTER COLUMN subject_id SET DEFAULT 'subject:intro-to-ml';
ALTER TABLE variants ALTER COLUMN subject_name_snapshot SET DEFAULT 'Введение в ML';
