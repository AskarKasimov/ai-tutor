-- name: EnqueueMaterialOCR :exec
INSERT INTO material_ocr_jobs(material_id, updated_at) VALUES ($1, $2);

-- name: ListMaterialOCRProgress :many
SELECT j.material_id, j.status, j.stage, j.attempts, j.updated_at
FROM material_ocr_jobs j JOIN subject_materials m ON m.id=j.material_id
WHERE m.subject_id=$1 ORDER BY j.updated_at, j.material_id;

-- name: GetMaterialOCRProgress :one
SELECT j.material_id, j.status, j.stage, j.attempts, j.updated_at
FROM material_ocr_jobs j WHERE j.material_id=$1;

-- name: ClaimMaterialOCR :one
UPDATE material_ocr_jobs j
SET status = 'processing', stage='ocr', attempts = attempts + CASE WHEN status='pending' THEN 1 ELSE 0 END,
    lease_until = sqlc.arg(now)::bigint + 60, lease_token = sqlc.arg(lease_token), updated_at = sqlc.arg(now)::bigint
WHERE j.material_id = (
    SELECT j.material_id FROM material_ocr_jobs j
    JOIN subject_materials m ON m.id=j.material_id
    WHERE ((j.status = 'pending' AND j.available_at <= sqlc.arg(now)::bigint)
       OR (j.status = 'processing' AND j.lease_until <= sqlc.arg(now)::bigint))
      AND (sqlc.arg(allow_pdf)::bool OR lower(m.filename) NOT LIKE '%.pdf')
    ORDER BY j.updated_at, j.material_id FOR UPDATE OF j SKIP LOCKED LIMIT 1
)
RETURNING j.*;

-- name: RenewMaterialOCR :execrows
UPDATE material_ocr_jobs SET lease_until=sqlc.arg(now)::bigint+60, updated_at=sqlc.arg(now)
WHERE material_id=sqlc.arg(material_id) AND lease_token=sqlc.arg(lease_token) AND status='processing';

-- name: SetMaterialOCRStage :execrows
UPDATE material_ocr_jobs SET stage=sqlc.arg(stage), updated_at=sqlc.arg(now)
WHERE material_id=sqlc.arg(material_id) AND lease_token=sqlc.arg(lease_token) AND status='processing';

-- name: ReleaseMaterialOCR :execrows
UPDATE material_ocr_jobs SET status='pending', attempts=GREATEST(attempts-1,0), lease_until=0,
    lease_token='', updated_at=sqlc.arg(now)
WHERE material_id=sqlc.arg(material_id) AND lease_token=sqlc.arg(lease_token) AND status='processing';

-- name: FinishMaterialOCR :execrows
UPDATE material_ocr_jobs
SET status = 'ready', stage='ready', error_message = '', lease_until = 0, updated_at = $3
WHERE material_id = $1 AND lease_token = $2 AND status = 'processing';

-- name: FailMaterialOCR :exec
WITH failed AS (
    UPDATE material_ocr_jobs
    SET status = CASE WHEN attempts >= 3 THEN 'failed' ELSE 'pending' END,
        error_message = sqlc.arg(error_message),
        available_at = sqlc.arg(available_at), lease_until = 0, updated_at = sqlc.arg(now)
    WHERE material_id = sqlc.arg(material_id) AND lease_token = sqlc.arg(lease_token) AND status = 'processing'
    RETURNING material_id, status, error_message
)
UPDATE subject_materials m
SET status = 'failed', error_message = failed.error_message, updated_at = sqlc.arg(now)
FROM failed WHERE m.id = failed.material_id AND failed.status = 'failed';

-- name: UpdateSubjectMaterialOCR :execrows
UPDATE subject_materials
SET s3_parsed_key = sqlc.arg(s3_parsed_key), s3_ocr_key=sqlc.arg(s3_ocr_key),
    ocr_report=sqlc.arg(ocr_report)::jsonb, parsed_text = sqlc.arg(parsed_text),
    char_count = sqlc.arg(char_count), status = 'parsed', error_message = sqlc.narg(error_message),
    updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(material_id);
