-- +goose Up
ALTER TABLE competency_map_imports ALTER COLUMN subject_id DROP DEFAULT;

-- +goose Down
ALTER TABLE competency_map_imports ALTER COLUMN subject_id SET DEFAULT 'subject:intro-to-ml';
