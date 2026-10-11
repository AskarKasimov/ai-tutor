-- name: ReadTrainingChat :one
SELECT revision, status, latest_check_data
FROM training_chats WHERE session_id=$1 AND exercise_id=$2;

-- name: ReadTrainingExercise :one
SELECT exercise_data FROM training_exercises e
JOIN training_sessions s ON s.id=e.session_id
WHERE s.owner_id=$1 AND e.session_id=$2 AND e.id=$3;

-- name: LockTrainingChat :one
SELECT revision, status, latest_check_data
FROM training_chats WHERE session_id=$1 AND exercise_id=$2 FOR UPDATE;

-- name: InsertTrainingChat :exec
INSERT INTO training_chats(exercise_id,session_id) VALUES($1,$2)
ON CONFLICT (exercise_id) DO NOTHING;

-- name: SetTrainingChatLatestCheck :exec
UPDATE training_chats SET revision=$3,latest_check_data=COALESCE($4::jsonb,latest_check_data)
WHERE session_id=$1 AND exercise_id=$2 AND status='active';

-- name: CloseTrainingChat :exec
UPDATE training_chats SET status='closed'
WHERE session_id=$1 AND exercise_id=$2;

-- name: InsertTrainingChatMessage :exec
INSERT INTO training_chat_messages(id,exercise_id,sequence,role,kind,source,text,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,to_timestamp($8));

-- name: ReadTrainingChatPage :many
SELECT id,sequence,role,kind,source,text,extract(epoch from created_at)::bigint AS created_at
FROM training_chat_messages
WHERE exercise_id=$1 AND ($2::bigint=0 OR sequence<$2)
ORDER BY sequence DESC LIMIT $3;

-- name: ReadTrainingChatRecent :many
SELECT id,sequence,role,kind,source,text,extract(epoch from created_at)::bigint AS created_at
FROM training_chat_messages WHERE exercise_id=$1
ORDER BY sequence DESC LIMIT $2;

-- name: ReadTrainingChatSummaryMessages :many
SELECT message.sequence,left(message.text,160) AS text FROM training_chat_messages AS message
WHERE message.exercise_id=$1 AND message.role='user' AND message.kind<>'welcome'
 AND message.sequence < COALESCE((SELECT min(recent.sequence) FROM (
     SELECT recent_messages.sequence FROM training_chat_messages AS recent_messages WHERE recent_messages.exercise_id=$1 ORDER BY recent_messages.sequence DESC LIMIT 24
 ) AS recent),0)
ORDER BY message.sequence DESC LIMIT 100;

-- name: FindTrainingChatTurnByKey :one
SELECT id,session_id,exercise_id,request_key,operation,fingerprint,expected_revision,token,
       lease_until,status,request_data,pending_audio,transcription_id,transcription_text,
       response_data,error_code,prompt_version,extract(epoch from created_at)::bigint AS created_at
FROM training_chat_turns WHERE session_id=$1 AND request_key=$2;

-- name: LockTrainingChatTurn :one
SELECT id,session_id,exercise_id,request_key,operation,fingerprint,expected_revision,token,
       lease_until,status,request_data,pending_audio,transcription_id,transcription_text,
       response_data,error_code,prompt_version,extract(epoch from created_at)::bigint AS created_at
FROM training_chat_turns WHERE session_id=$1 AND exercise_id=$2 AND id=$3 FOR UPDATE;

-- name: FindTrainingChatPendingTurn :one
SELECT id,session_id,exercise_id,request_key,operation,fingerprint,expected_revision,token,
       lease_until,status,request_data,pending_audio,transcription_id,transcription_text,
       response_data,error_code,prompt_version,extract(epoch from created_at)::bigint AS created_at
FROM training_chat_turns WHERE session_id=$1 AND exercise_id=$2
 AND status IN ('processing','failed') ORDER BY created_at DESC LIMIT 1;

-- name: InsertTrainingChatTurn :exec
INSERT INTO training_chat_turns(id,session_id,exercise_id,request_key,operation,fingerprint,
 expected_revision,token,lease_until,status,request_data,pending_audio)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,now()+$9::double precision*interval '1 second','processing',$10,$11);

-- name: RenewTrainingChatTurn :execrows
UPDATE training_chat_turns SET token=$4,lease_until=now()+$5::double precision*interval '1 second',
 status='processing',error_code=NULL,updated_at=now()
WHERE session_id=$1 AND exercise_id=$2 AND id=$3 AND status IN ('processing','failed')
 AND (status='failed' OR lease_until IS NULL OR lease_until<=now());

-- name: SaveTrainingChatTranscript :execrows
UPDATE training_chat_turns SET transcription_id=$4,transcription_text=$5,pending_audio=NULL,updated_at=now()
WHERE session_id=$1 AND exercise_id=$2 AND id=$3 AND token=$6 AND status='processing';

-- name: FailTrainingChatTurn :execrows
UPDATE training_chat_turns SET status='failed',lease_until=NULL,error_code=$4,updated_at=now()
WHERE session_id=$1 AND exercise_id=$2 AND id=$3 AND token=$5 AND status='processing';

-- name: CancelTrainingChatTurn :execrows
UPDATE training_chat_turns SET status='cancelled',token=$4,lease_until=NULL,pending_audio=NULL,
 error_code=NULL,updated_at=now()
WHERE session_id=$1 AND exercise_id=$2 AND id=$3 AND status IN ('processing','failed');

-- name: CompleteTrainingChatTurn :execrows
UPDATE training_chat_turns SET status='completed',lease_until=NULL,pending_audio=NULL,
	response_data=$4,prompt_version=$5,error_code=NULL,updated_at=now(),check_data=$7
WHERE session_id=$1 AND exercise_id=$2 AND id=$3 AND token=$6 AND status='processing';
