-- name: EnqueueTaskAudio :exec
INSERT INTO audio_assets(id, instruction, object_key)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO NOTHING;

-- name: GetTaskAudio :one
SELECT id, instruction, object_key, bucket, storage_uri, audio_url, status, attempts
FROM audio_assets WHERE id = $1;

-- name: GetAccessibleTaskAudio :one
SELECT asset.id, asset.instruction, asset.object_key, asset.bucket, asset.storage_uri,
       asset.audio_url, asset.status, asset.attempts
FROM audio_assets asset
WHERE asset.id = $2 AND (
    EXISTS (SELECT 1 FROM tasks task WHERE task.audio_asset_id = asset.id)
    OR EXISTS (
        SELECT 1 FROM variant_tasks variant_task
        JOIN variants variant ON variant.id = variant_task.variant_id
        WHERE variant.user_id = $1 AND variant_task.audio_asset_id = asset.id
    )
    OR EXISTS (
        SELECT 1 FROM training_exercises exercise
        JOIN training_sessions session ON session.id=exercise.session_id
        WHERE session.owner_id=$1 AND exercise.audio_asset_id=asset.id
          AND session.state_data->'Current'->>'ID'=exercise.id
    )
);

-- name: RecoverExpiredTaskAudio :exec
UPDATE audio_assets
SET status = CASE
        WHEN NOT EXISTS (
            SELECT 1
            FROM tasks task
            JOIN outcomes outcome ON outcome.id = task.outcome_id
            JOIN constituents constituent ON constituent.id = outcome.constituent_id
            JOIN competencies competency ON competency.id = constituent.competency_id
            JOIN subjects subject ON subject.active_revision = competency.revision
            WHERE task.audio_asset_id = audio_assets.id
        ) THEN 'cancelled'
        WHEN attempts >= 5 THEN 'failed'
        ELSE 'pending'
    END,
    lease_until = NULL,
    claim_token = NULL,
    next_attempt_at = CASE WHEN attempts >= 5 THEN next_attempt_at ELSE now() END,
    last_error_code = CASE
        WHEN NOT EXISTS (
            SELECT 1
            FROM tasks task
            JOIN outcomes outcome ON outcome.id = task.outcome_id
            JOIN constituents constituent ON constituent.id = outcome.constituent_id
            JOIN competencies competency ON competency.id = constituent.competency_id
            JOIN subjects subject ON subject.active_revision = competency.revision
            WHERE task.audio_asset_id = audio_assets.id
        ) THEN 'task_no_longer_current'
        WHEN attempts >= 5 THEN 'attempts_exhausted'
        ELSE last_error_code
    END,
    updated_at = now()
WHERE status = 'processing' AND lease_until <= now();

-- name: CancelPendingTaskAudioForSubject :execrows
UPDATE audio_assets asset
SET status = 'cancelled', last_error_code = 'task_no_longer_current', updated_at = now()
WHERE asset.status = 'pending'
  AND EXISTS (
      SELECT 1
      FROM tasks task
      JOIN outcomes outcome ON outcome.id = task.outcome_id
      JOIN constituents constituent ON constituent.id = outcome.constituent_id
      JOIN competencies competency ON competency.id = constituent.competency_id
      JOIN subjects subject ON subject.active_revision = competency.revision
      WHERE task.audio_asset_id = asset.id AND subject.id = $1
  );

-- name: ClaimTaskAudio :one
WITH candidate AS (
    SELECT id FROM audio_assets
    WHERE status = 'pending' AND next_attempt_at <= now() AND attempts < 5
      AND EXISTS (
          SELECT 1
          FROM tasks task
          JOIN outcomes outcome ON outcome.id = task.outcome_id
          JOIN constituents constituent ON constituent.id = outcome.constituent_id
          JOIN competencies competency ON competency.id = constituent.competency_id
          JOIN subjects subject ON subject.active_revision = competency.revision
          WHERE task.audio_asset_id = audio_assets.id
      )
    ORDER BY created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE audio_assets asset
SET status = 'processing',
    attempts = asset.attempts + 1,
    claim_token = sqlc.arg(claim_token),
    lease_until = now() + sqlc.arg(lease_seconds)::double precision * interval '1 second',
    updated_at = now()
FROM candidate
WHERE asset.id = candidate.id
RETURNING asset.id, asset.instruction, asset.object_key, asset.bucket, asset.storage_uri,
          asset.audio_url, asset.status, asset.attempts;

-- name: ClaimTaskAudioForRepair :one
WITH candidate AS MATERIALIZED (
    SELECT asset.id, asset.bucket AS previous_bucket
    FROM audio_assets asset
    WHERE asset.id = sqlc.arg(id)
      AND asset.status IN ('ready', 'failed')
      AND EXISTS (
      SELECT 1
      FROM tasks task
      JOIN outcomes outcome ON outcome.id = task.outcome_id
      JOIN constituents constituent ON constituent.id = outcome.constituent_id
      JOIN competencies competency ON competency.id = constituent.competency_id
      JOIN subjects subject ON subject.active_revision = competency.revision
      WHERE task.audio_asset_id = asset.id
      )
    FOR UPDATE
), claimed AS (
    UPDATE audio_assets asset
    SET status = 'processing',
        attempts = 1,
        bucket = NULL,
        storage_uri = NULL,
        audio_url = NULL,
        claim_token = sqlc.arg(claim_token),
        lease_until = now() + sqlc.arg(lease_seconds)::double precision * interval '1 second',
        updated_at = now()
    FROM candidate
    WHERE asset.id = candidate.id AND asset.status IN ('ready', 'failed')
    RETURNING asset.id, asset.instruction, asset.object_key, asset.bucket, asset.storage_uri,
              asset.audio_url, asset.status, asset.attempts, candidate.previous_bucket
)
SELECT * FROM claimed;

-- name: IsCurrentTaskAudio :one
SELECT EXISTS (
    SELECT 1
    FROM audio_assets asset
    JOIN tasks task ON task.audio_asset_id = asset.id
    JOIN outcomes outcome ON outcome.id = task.outcome_id
    JOIN constituents constituent ON constituent.id = outcome.constituent_id
    JOIN competencies competency ON competency.id = constituent.competency_id
    JOIN subjects subject ON subject.active_revision = competency.revision
    WHERE asset.id = sqlc.arg(id)
      AND asset.status = 'processing'
      AND asset.claim_token = sqlc.arg(claim_token)
      AND asset.lease_until > now()
);

-- name: CancelClaimedTaskAudio :execrows
UPDATE audio_assets
SET status = 'cancelled', lease_until = NULL, claim_token = NULL,
    last_error_code = 'task_no_longer_current', updated_at = now()
WHERE audio_assets.id = sqlc.arg(id) AND status = 'processing' AND claim_token = sqlc.arg(claim_token)
  AND NOT EXISTS (
      SELECT 1
      FROM tasks task
      JOIN outcomes outcome ON outcome.id = task.outcome_id
      JOIN constituents constituent ON constituent.id = outcome.constituent_id
      JOIN competencies competency ON competency.id = constituent.competency_id
      JOIN subjects subject ON subject.active_revision = competency.revision
      WHERE task.audio_asset_id = audio_assets.id
  );

-- name: CompleteTaskAudio :execrows
UPDATE audio_assets
SET status = 'ready', bucket = $3, storage_uri = $4, audio_url = $5,
    lease_until = NULL, claim_token = NULL, last_error_code = NULL, updated_at = now()
WHERE id = $1 AND status = 'processing' AND claim_token = $2;

-- name: FailTaskAudio :execrows
UPDATE audio_assets
SET status = CASE
        WHEN NOT EXISTS (
            SELECT 1
            FROM tasks task
            JOIN outcomes outcome ON outcome.id = task.outcome_id
            JOIN constituents constituent ON constituent.id = outcome.constituent_id
            JOIN competencies competency ON competency.id = constituent.competency_id
            JOIN subjects subject ON subject.active_revision = competency.revision
            WHERE task.audio_asset_id = audio_assets.id
        ) THEN 'cancelled'
        WHEN attempts >= 5 THEN 'failed'
        ELSE 'pending'
    END,
    next_attempt_at = CASE
        WHEN attempts >= 5 OR NOT EXISTS (
            SELECT 1
            FROM tasks task
            JOIN outcomes outcome ON outcome.id = task.outcome_id
            JOIN constituents constituent ON constituent.id = outcome.constituent_id
            JOIN competencies competency ON competency.id = constituent.competency_id
            JOIN subjects subject ON subject.active_revision = competency.revision
            WHERE task.audio_asset_id = audio_assets.id
        ) THEN next_attempt_at
        ELSE now() + sqlc.arg(delay_seconds)::double precision * interval '1 second'
    END,
    lease_until = NULL, claim_token = NULL,
    last_error_code = CASE
        WHEN NOT EXISTS (
            SELECT 1
            FROM tasks task
            JOIN outcomes outcome ON outcome.id = task.outcome_id
            JOIN constituents constituent ON constituent.id = outcome.constituent_id
            JOIN competencies competency ON competency.id = constituent.competency_id
            JOIN subjects subject ON subject.active_revision = competency.revision
            WHERE task.audio_asset_id = audio_assets.id
        ) THEN 'task_no_longer_current'
        ELSE sqlc.arg(error_code)
    END,
    updated_at = now()
WHERE audio_assets.id = sqlc.arg(id) AND status = 'processing' AND claim_token = sqlc.arg(claim_token);

-- name: FindReadyTaskAudioByInstruction :one
-- A ready recording of the same instruction can be copied instead of synthesized.
SELECT id, instruction, object_key, bucket, storage_uri, audio_url, status, attempts
FROM audio_assets
WHERE instruction = $1 AND id <> $2 AND status = 'ready'
ORDER BY updated_at DESC, id
LIMIT 1;

-- name: InsertTaskAudioAsset :one
INSERT INTO audio_assets(id, instruction, object_key)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO UPDATE SET instruction = audio_assets.instruction
RETURNING instruction;

-- name: LinkTaskAudioAsset :execrows
UPDATE tasks SET audio_asset_id = $2 WHERE id = $1;
