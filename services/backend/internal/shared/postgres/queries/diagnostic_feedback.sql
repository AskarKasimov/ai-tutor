-- name: ReadDiagnosticFeedback :one
SELECT feedback_data FROM diagnostic_feedback WHERE owner_id = $1 AND session_id = $2;

-- name: SaveDiagnosticFeedback :exec
INSERT INTO diagnostic_feedback(session_id, owner_id, feedback_data) VALUES ($1, $2, $3)
ON CONFLICT (session_id) DO NOTHING;
