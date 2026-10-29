-- name: LearningReadGraphLabels :many
SELECT cache_key, display_name FROM learning_graph_labels WHERE cache_key = ANY($1::text[]);

-- name: LearningSaveGraphLabel :exec
INSERT INTO learning_graph_labels (cache_key, display_name) VALUES ($1, $2)
ON CONFLICT (cache_key) DO NOTHING;
