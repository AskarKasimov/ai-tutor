-- name: LearningReadDiagnosticFeedback :one
SELECT feedback_data FROM learning_diagnostic_feedback WHERE owner_id = $1 AND session_id = $2;

-- name: LearningSaveDiagnosticFeedback :exec
INSERT INTO learning_diagnostic_feedback(session_id, owner_id, feedback_data) VALUES ($1, $2, $3)
ON CONFLICT (session_id) DO UPDATE SET feedback_data = EXCLUDED.feedback_data
WHERE learning_diagnostic_feedback.owner_id = EXCLUDED.owner_id
  AND COALESCE((learning_diagnostic_feedback.feedback_data->>'analysis_version')::int, 0)
      < COALESCE((EXCLUDED.feedback_data->>'analysis_version')::int, 0);
