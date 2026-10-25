-- name: LearningFindDiagnosticSessionByStart :one
SELECT session_data, start_request_digest
FROM learning_diagnostic_sessions
WHERE owner_id = sqlc.arg(owner_id)::text
  AND start_request_key = sqlc.arg(start_request_key)::text;

-- name: LearningGetDiagnosticSession :one
SELECT session_data
FROM learning_diagnostic_sessions
WHERE owner_id = sqlc.arg(owner_id)::text
  AND id = sqlc.arg(id)::text;

-- name: LearningCreateDiagnosticSession :execrows
INSERT INTO learning_diagnostic_sessions(
    id, owner_id, variant_id, subject_id, subject_name_snapshot, map_revision, status,
    current_competency, current_task, start_request_key, start_request_digest, session_data
)
SELECT sqlc.arg(id)::text, sqlc.arg(owner_id)::text, variant.id, variant.subject_id,
       variant.subject_name_snapshot, variant.map_revision, sqlc.arg(status)::text,
       sqlc.arg(current_competency)::integer, sqlc.arg(current_task)::integer,
       sqlc.arg(start_request_key)::text, sqlc.arg(start_request_digest)::text,
       sqlc.arg(session_data)::jsonb
FROM learning_variants AS variant
WHERE variant.id = sqlc.arg(variant_id)::text
  AND variant.user_id = sqlc.arg(owner_id)::text
ON CONFLICT (owner_id, start_request_key) DO NOTHING;
-- name: LearningReadSubjectLearningState :one
SELECT subject.id,subject.name,
 COALESCE((SELECT id FROM learning_diagnostic_sessions d WHERE d.owner_id=sqlc.arg(owner_id)::text AND d.subject_id=subject.id AND d.status='completed' ORDER BY d.completed_at DESC NULLS LAST,d.id DESC LIMIT 1),'')::text AS completed_id,
 COALESCE((SELECT id FROM learning_diagnostic_sessions d WHERE d.owner_id=sqlc.arg(owner_id)::text AND d.subject_id=subject.id AND d.status='active' ORDER BY d.created_at DESC,d.id DESC LIMIT 1),'')::text AS active_id
FROM subjects subject WHERE subject.id=sqlc.arg(subject_id)::text;

-- name: LearningLatestCompletedDiagnostic :one
SELECT session_data FROM learning_diagnostic_sessions WHERE owner_id=$1 AND subject_id=$2 AND status='completed' ORDER BY completed_at DESC NULLS LAST,id DESC LIMIT 1;
-- name: LearningLockDiagnosticSession :one
SELECT session_data,COALESCE(lease_until>now(),false)::boolean AS leased FROM learning_diagnostic_sessions WHERE owner_id=$1 AND id=$2 FOR UPDATE;
-- name: LearningSaveDiagnosticSession :exec
UPDATE learning_diagnostic_sessions SET session_data=$2,status=$3,current_competency=$4,current_task=$5,
updated_at=now(),lease_until=CASE WHEN sqlc.arg(lease_seconds)::bigint>0 THEN now()+sqlc.arg(lease_seconds)::bigint*interval '1 second' ELSE NULL END,
completed_at=CASE WHEN $3='completed' THEN COALESCE(completed_at,now()) ELSE completed_at END WHERE id=$1;
-- name: LearningInsertDiagnosticAnswer :exec
INSERT INTO learning_diagnostic_answers(session_id,answer_order,idempotency_key,request_digest,variant_task_id,answer_data,progress_data) VALUES($1,$2,$3,$4,$5,$6,$7);
