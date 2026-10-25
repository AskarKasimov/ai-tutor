-- name: GeekSubjectExists :one
SELECT EXISTS (SELECT 1 FROM subjects WHERE id = $1 AND mode = 'exam');

-- name: GetGeekSession :one
SELECT state FROM geek_sessions WHERE id = $1 AND owner_id = $2;

-- name: GetActiveGeekSession :one
SELECT state FROM geek_sessions
WHERE owner_id = $1 AND subject_id = $2 AND status = 'active';

-- name: InsertGeekSession :exec
INSERT INTO geek_sessions(id, owner_id, subject_id, status, version, started_at, completed_at, updated_at, state)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: UpdateGeekSession :execrows
UPDATE geek_sessions
SET status = sqlc.arg(status), version = sqlc.arg(version), completed_at = sqlc.arg(completed_at),
    updated_at = sqlc.arg(updated_at), state = sqlc.arg(state)
WHERE id = sqlc.arg(id) AND version = sqlc.arg(previous_version);

-- name: GetGeekRequest :one
SELECT fingerprint, session_id, created, response, created_at
FROM geek_requests WHERE owner_id = $1 AND key = $2;

-- name: InsertGeekRequest :exec
INSERT INTO geek_requests(owner_id, key, fingerprint, session_id, created, response, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (owner_id, key) DO NOTHING;

-- name: GetOwnedTranscriptionText :one
SELECT text FROM transcriptions WHERE id = $1 AND user_id = $2;

-- name: ListOwnGeekSessions :many
SELECT state FROM geek_sessions
WHERE owner_id = $1 AND subject_id = $2
ORDER BY started_at DESC, id DESC
LIMIT 50;
