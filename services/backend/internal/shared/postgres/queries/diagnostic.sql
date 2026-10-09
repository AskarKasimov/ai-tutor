-- name: FindDiagnosticSessionByStart :one
SELECT session_data, start_request_digest
FROM diagnostic_sessions
WHERE owner_id = sqlc.arg(owner_id)::text
  AND start_request_key = sqlc.arg(start_request_key)::text;

-- name: GetDiagnosticSession :one
SELECT session_data
FROM diagnostic_sessions
WHERE owner_id = sqlc.arg(owner_id)::text
  AND id = sqlc.arg(id)::text;

-- name: CreateDiagnosticSession :execrows
INSERT INTO diagnostic_sessions(
    id, owner_id, variant_id, subject_id, subject_name_snapshot, map_revision, status,
    current_competency, current_task, start_request_key, start_request_digest, session_data
)
SELECT sqlc.arg(id)::text, sqlc.arg(owner_id)::text, variant.id, variant.subject_id,
       variant.subject_name_snapshot, variant.map_revision, sqlc.arg(status)::text,
       sqlc.arg(current_competency)::integer, sqlc.arg(current_task)::integer,
       sqlc.arg(start_request_key)::text, sqlc.arg(start_request_digest)::text,
       sqlc.arg(session_data)::jsonb
FROM variants AS variant
WHERE variant.id = sqlc.arg(variant_id)::text
  AND variant.user_id = sqlc.arg(owner_id)::text
ON CONFLICT (owner_id, start_request_key) DO NOTHING;
