-- +goose Up
ALTER TABLE audio_assets ADD COLUMN priority_requested_at timestamptz;

-- +goose Down
ALTER TABLE audio_assets DROP COLUMN priority_requested_at;
