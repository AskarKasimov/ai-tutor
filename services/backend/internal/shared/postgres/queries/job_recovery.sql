-- Queries of cmd/recover-failed-jobs. A filter argument of '' matches every
-- row; stalled also selects processing jobs whose lease has expired.

-- name: ListRecoverableOCR :many
SELECT j.material_id, m.subject_id, j.status, j.stage, j.attempts, j.lease_token, j.error_message
FROM material_ocr_jobs j JOIN subject_materials m ON m.id = j.material_id
WHERE (sqlc.arg(material_id)::text = '' OR j.material_id = sqlc.arg(material_id)::text)
  AND (sqlc.arg(subject_id)::text = '' OR m.subject_id = sqlc.arg(subject_id)::text)
  AND (j.status = 'failed' OR (sqlc.arg(stalled)::bool AND j.status = 'processing' AND j.lease_until <= sqlc.arg(now)::bigint))
ORDER BY j.updated_at, j.material_id;

-- name: RecoverOCR :execrows
-- Resets a failed job, or a stalled one whose token matches (token '' takes
-- any), and returns its failed material to pending.
WITH recovered AS (
    UPDATE material_ocr_jobs
    SET status = 'pending', stage = 'queued', attempts = 0, error_message = '', available_at = sqlc.arg(now)::bigint,
        lease_until = 0, lease_token = '', updated_at = sqlc.arg(now)::bigint
    WHERE material_id = sqlc.arg(material_id)::text
      AND (status = 'failed' OR (status = 'processing' AND lease_until <= sqlc.arg(now)::bigint
           AND (sqlc.arg(token)::text = '' OR lease_token = sqlc.arg(token)::text)))
    RETURNING material_id
), material AS (
    UPDATE subject_materials m SET status = 'pending', error_message = NULL, updated_at = sqlc.arg(now)::bigint
    FROM recovered r WHERE m.id = r.material_id AND m.status = 'failed'
)
SELECT material_id FROM recovered;

-- name: ListRecoverableCleaning :many
SELECT j.material_id, m.subject_id, j.status, j.attempts, j.lease_token, j.error_message
FROM material_cleaning_jobs j JOIN subject_materials m ON m.id = j.material_id
WHERE (sqlc.arg(material_id)::text = '' OR j.material_id = sqlc.arg(material_id)::text)
  AND (sqlc.arg(subject_id)::text = '' OR m.subject_id = sqlc.arg(subject_id)::text)
  AND (j.status = 'failed' OR (sqlc.arg(stalled)::bool AND j.status = 'processing' AND j.lease_until <= sqlc.arg(now)::bigint))
ORDER BY j.updated_at, j.material_id;

-- name: RecoverCleaning :execrows
UPDATE material_cleaning_jobs
SET status = 'pending', attempts = 0, error_message = '', available_at = sqlc.arg(now)::bigint,
    lease_until = 0, lease_token = '', updated_at = sqlc.arg(now)::bigint
WHERE material_id = sqlc.arg(material_id)::text
  AND (status = 'failed' OR (status = 'processing' AND lease_until <= sqlc.arg(now)::bigint
       AND (sqlc.arg(token)::text = '' OR lease_token = sqlc.arg(token)::text)));

-- name: ListRecoverableCourseGraphs :many
-- material_id limits the graph to the course of that material.
SELECT j.subject_id, j.status, j.attempts, j.lease_token, j.error_message
FROM course_graph_jobs j
WHERE (sqlc.arg(subject_id)::text = '' OR j.subject_id = sqlc.arg(subject_id)::text)
  AND (sqlc.arg(material_id)::text = '' OR j.subject_id = (SELECT m.subject_id FROM subject_materials m WHERE m.id = sqlc.arg(material_id)::text))
  AND (j.status = 'failed' OR (sqlc.arg(stalled)::bool AND j.status = 'processing' AND j.lease_until <= sqlc.arg(now)::bigint))
ORDER BY j.updated_at, j.subject_id;

-- name: RecoverCourseGraph :execrows
UPDATE course_graph_jobs
SET status = 'pending', attempts = 0, error_message = '', available_at = sqlc.arg(now)::bigint,
    lease_until = 0, lease_token = '', updated_at = sqlc.arg(now)::bigint
WHERE subject_id = sqlc.arg(subject_id)::text
  AND (status = 'failed' OR (status = 'processing' AND lease_until <= sqlc.arg(now)::bigint
       AND (sqlc.arg(token)::text = '' OR lease_token = sqlc.arg(token)::text)));
