-- name: InsertTranscription :exec
INSERT INTO transcriptions(id, user_id, text, created_at) VALUES ($1, $2, $3, $4);

-- name: GetTranscriptionByOwner :one
SELECT text FROM transcriptions WHERE id = $1 AND user_id = $2;
