-- name: ReadTrainingSession :one
SELECT state_data FROM training_sessions WHERE owner_id=$1 AND id=$2;
-- name: FindTrainingByDiagnostic :one
SELECT state_data FROM training_sessions WHERE owner_id=$1 AND diagnostic_id=$2;
-- name: FindTrainingStartKey :one
SELECT diagnostic_id,session_id FROM training_start_requests WHERE owner_id=$1 AND request_key=$2;
-- name: LockTrainingSession :one
SELECT state_data,reservation_data,COALESCE(lease_until>now(),false)::boolean AS leased
FROM training_sessions WHERE owner_id=$1 AND id=$2 FOR UPDATE;
-- name: LockTrainingDiagnostic :one
SELECT status FROM diagnostic_sessions WHERE owner_id=$1 AND id=$2 FOR UPDATE;
-- name: InsertTrainingSession :exec
INSERT INTO training_sessions(id,owner_id,diagnostic_id,subject_id,state_data) VALUES($1,$2,$3,$4,$5);
-- name: InsertTrainingStartKey :exec
INSERT INTO training_start_requests(owner_id,request_key,diagnostic_id,session_id) VALUES($1,$2,$3,$4);
-- name: InsertTrainingTarget :exec
INSERT INTO training_targets(session_id,position,target_data) VALUES($1,$2,$3);
-- name: InsertTrainingExercise :exec
INSERT INTO training_exercises(id,session_id,round,target_index,exercise_data,audio_asset_id) VALUES($1,$2,$3,$4,$5,$6);
-- name: SaveTrainingReservation :exec
UPDATE training_sessions SET reservation_data=$3,lease_until=CASE WHEN sqlc.arg(leased)::boolean THEN now()+interval '5 minutes' ELSE NULL END WHERE owner_id=$1 AND id=$2;

-- name: ResetTrainingReservation :exec
UPDATE training_sessions SET reservation_data=NULL,lease_until=NULL WHERE owner_id=$1 AND id=$2;
-- name: SaveTrainingState :exec
UPDATE training_sessions SET state_data=$3,reservation_data=NULL,lease_until=NULL WHERE owner_id=$1 AND id=$2;
-- name: FindTrainingAnswer :one
SELECT fingerprint,response_data FROM training_attempts WHERE session_id=$1 AND request_key=$2;
-- name: InsertTrainingAttempt :exec
INSERT INTO training_attempts(session_id,sequence,request_key,fingerprint,exercise_id,attempt_data,response_data) VALUES($1,$2,$3,$4,$5,$6,$7);
-- name: ReadTrainingHistory :many
SELECT attempt_data,sequence FROM training_attempts WHERE session_id=$1 AND sequence<sqlc.arg(before_sequence)::bigint ORDER BY sequence DESC LIMIT sqlc.arg(page_size)::integer;
-- name: LockTrainingOwner :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(owner_id)::text, 901));
