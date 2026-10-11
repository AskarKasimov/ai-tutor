-- +goose Up
-- A competency block issues several exercises per round: main, theory and basics.
ALTER TABLE training_exercises DROP CONSTRAINT IF EXISTS training_exercises_session_id_round_target_index_key;
CREATE INDEX training_exercises_session_idx ON training_exercises (session_id, round, target_index);

-- +goose Down
DROP INDEX training_exercises_session_idx;
ALTER TABLE training_exercises ADD CONSTRAINT training_exercises_session_id_round_target_index_key UNIQUE (session_id, round, target_index);
