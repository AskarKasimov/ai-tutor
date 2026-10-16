-- name: EnqueueMaterialCleaning :exec
INSERT INTO material_cleaning_jobs(material_id, updated_at) VALUES ($1, $2);

-- name: ListMaterialCleaning :many
SELECT j.* FROM material_cleaning_jobs j
JOIN subject_materials m ON m.id = j.material_id WHERE m.subject_id = $1;

-- name: GetMaterialCleaning :one
SELECT * FROM material_cleaning_jobs WHERE material_id = $1;

-- name: ClaimMaterialCleaning :one
UPDATE material_cleaning_jobs j
SET status = 'processing', attempts = attempts + 1,
    lease_until = sqlc.arg(lease_until), lease_token = sqlc.arg(lease_token), updated_at = sqlc.arg(now)::bigint
WHERE j.material_id = (
    SELECT material_id FROM material_cleaning_jobs
    WHERE (status = 'pending' AND available_at <= sqlc.arg(now)::bigint)
       OR (status = 'processing' AND lease_until <= sqlc.arg(now)::bigint)
    ORDER BY updated_at, material_id FOR UPDATE SKIP LOCKED LIMIT 1
)
RETURNING j.*;

-- name: FinishMaterialCleaning :execrows
UPDATE material_cleaning_jobs
SET status = 'ready', cleaned_text = $3, s3_cleaned_key = $4, s3_report_key = $5,
    error_message = '', lease_until = 0, updated_at = $6
WHERE material_id = $1 AND lease_token = $2 AND status = 'processing';

-- name: FailMaterialCleaning :execrows
UPDATE material_cleaning_jobs
SET status = CASE WHEN attempts >= 3 THEN 'failed' ELSE 'pending' END,
    error_message = $3, available_at = $4, lease_until = 0, updated_at = $5
WHERE material_id = $1 AND lease_token = $2 AND status = 'processing';
