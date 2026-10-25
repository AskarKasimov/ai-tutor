-- name: LearningReadTrainingSession :one
SELECT state_data FROM learning_training_sessions WHERE owner_id=$1 AND id=$2;
-- name: LearningFindTrainingByDiagnostic :one
SELECT state_data FROM learning_training_sessions WHERE owner_id=$1 AND diagnostic_id=$2;
-- name: LearningFindTrainingStartKey :one
SELECT diagnostic_id,session_id FROM learning_training_start_requests WHERE owner_id=$1 AND request_key=$2;
-- name: LearningLockTrainingSession :one
SELECT state_data,reservation_data,COALESCE(lease_until>now(),false)::boolean AS leased
FROM learning_training_sessions WHERE owner_id=$1 AND id=$2 FOR UPDATE;
-- name: LearningLockTrainingDiagnostic :one
SELECT status FROM learning_diagnostic_sessions WHERE owner_id=$1 AND id=$2 FOR UPDATE;
-- name: LearningInsertTrainingSession :exec
INSERT INTO learning_training_sessions(id,owner_id,diagnostic_id,subject_id,state_data) VALUES($1,$2,$3,$4,$5);
-- name: LearningInsertTrainingStartKey :exec
INSERT INTO learning_training_start_requests(owner_id,request_key,diagnostic_id,session_id) VALUES($1,$2,$3,$4);
-- name: LearningInsertTrainingTarget :exec
INSERT INTO learning_training_targets(session_id,position,target_data) VALUES($1,$2,$3);
-- name: LearningInsertTrainingExercise :exec
INSERT INTO learning_training_exercises(id,session_id,round,target_index,exercise_data,audio_asset_id) VALUES($1,$2,$3,$4,$5,$6);
-- name: LearningSaveTrainingReservation :exec
UPDATE learning_training_sessions SET reservation_data=$3,lease_until=CASE WHEN sqlc.arg(leased)::boolean THEN now()+interval '5 minutes' ELSE NULL END WHERE owner_id=$1 AND id=$2;

-- name: LearningSaveTrainingReservationWithLease :exec
UPDATE learning_training_sessions SET reservation_data=$3,
 lease_until=CASE WHEN sqlc.arg(lease_seconds)::bigint>0 THEN now()+sqlc.arg(lease_seconds)::bigint*interval '1 second' ELSE NULL END
WHERE owner_id=$1 AND id=$2;

-- name: LearningResetTrainingReservation :exec
UPDATE learning_training_sessions SET reservation_data=NULL,lease_until=NULL WHERE owner_id=$1 AND id=$2;
-- name: LearningSaveTrainingState :exec
UPDATE learning_training_sessions SET state_data=$3,reservation_data=NULL,lease_until=NULL WHERE owner_id=$1 AND id=$2;
-- name: LearningFindTrainingAnswer :one
SELECT fingerprint,response_data FROM learning_training_attempts WHERE session_id=$1 AND request_key=$2;
-- name: LearningInsertTrainingAttempt :exec
INSERT INTO learning_training_attempts(session_id,sequence,request_key,fingerprint,exercise_id,attempt_data,response_data) VALUES($1,$2,$3,$4,$5,$6,$7);
-- name: LearningReadTrainingHistory :many
SELECT attempt_data,sequence FROM learning_training_attempts WHERE session_id=$1 AND sequence<sqlc.arg(before_sequence)::bigint ORDER BY sequence DESC LIMIT sqlc.arg(page_size)::integer;
-- name: LearningLockTrainingOwner :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(owner_id)::text, 901));
