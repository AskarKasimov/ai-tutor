-- name: SyncCourseGraphJobs :exec
INSERT INTO course_graph_jobs(subject_id, fingerprint, updated_at)
SELECT subject_id, fingerprint, sqlc.arg(now)::bigint FROM course_graph_inputs
ON CONFLICT(subject_id) DO UPDATE SET fingerprint=excluded.fingerprint,
    status='pending', attempts=0, available_at=0, lease_until=0, lease_token='', error_message='', updated_at=excluded.updated_at
WHERE course_graph_jobs.fingerprint <> excluded.fingerprint;

-- name: ClaimCourseGraph :one
UPDATE course_graph_jobs SET status='processing', attempts=attempts+CASE WHEN status='pending' THEN 1 ELSE 0 END,
    lease_token=sqlc.arg(token), lease_until=sqlc.arg(now)::bigint+60, updated_at=sqlc.arg(now)
WHERE subject_id=(SELECT j.subject_id FROM course_graph_jobs j JOIN course_graph_inputs i
    ON i.subject_id=j.subject_id AND i.fingerprint=j.fingerprint
    WHERE (j.status='pending' AND j.available_at<=sqlc.arg(now))
       OR (j.status='processing' AND j.lease_until<=sqlc.arg(now))
    ORDER BY j.updated_at FOR UPDATE OF j SKIP LOCKED LIMIT 1)
RETURNING *;

-- name: RenewCourseGraph :execrows
UPDATE course_graph_jobs SET lease_until=sqlc.arg(now)::bigint+60, updated_at=sqlc.arg(now)
WHERE subject_id=sqlc.arg(subject_id) AND lease_token=sqlc.arg(token) AND status='processing';

-- name: ReleaseCourseGraph :execrows
UPDATE course_graph_jobs SET status='pending', attempts=GREATEST(attempts-1,0), lease_until=0,
    lease_token='', updated_at=sqlc.arg(now)
WHERE subject_id=sqlc.arg(subject_id) AND lease_token=sqlc.arg(token) AND status='processing';

-- name: FinishCourseGraph :one
WITH accepted AS (
    UPDATE course_graph_jobs j SET status='ready', lease_until=0, error_message='', updated_at=sqlc.arg(now)
    WHERE j.subject_id=sqlc.arg(subject_id) AND j.lease_token=sqlc.arg(token) AND j.status='processing'
      AND j.fingerprint=sqlc.arg(fingerprint)
      AND EXISTS(SELECT 1 FROM course_graph_inputs i WHERE i.subject_id=sqlc.arg(subject_id) AND i.fingerprint=sqlc.arg(fingerprint))
    RETURNING j.subject_id, j.fingerprint
)
INSERT INTO course_graphs(subject_id,fingerprint,graph,updated_at)
SELECT accepted.subject_id,accepted.fingerprint,sqlc.arg(graph)::jsonb,sqlc.arg(now)::bigint FROM accepted
ON CONFLICT(subject_id) DO UPDATE SET fingerprint=excluded.fingerprint,graph=excluded.graph,updated_at=excluded.updated_at
RETURNING subject_id;

-- name: FailCourseGraph :exec
UPDATE course_graph_jobs SET status=CASE WHEN attempts>=3 THEN 'failed' ELSE 'pending' END,
    available_at=sqlc.arg(now)::bigint+30*attempts, lease_until=0, error_message=sqlc.arg(error_message),updated_at=sqlc.arg(now)
WHERE subject_id=sqlc.arg(subject_id) AND lease_token=sqlc.arg(token) AND status='processing';

-- name: ReadCourseGraph :one
SELECT s.id, coalesce(j.status,'waiting_materials')::text AS status,
       coalesce(j.attempts,0)::integer AS attempts, coalesce(j.error_message,'')::text AS error_message,
       coalesce(g.graph,'null'::jsonb)::jsonb AS graph,
       coalesce(g.updated_at,0)::bigint AS updated_at,
       (g.fingerprint IS DISTINCT FROM i.fingerprint OR i.fingerprint IS NULL)::boolean AS stale
FROM subjects s LEFT JOIN course_graph_jobs j ON j.subject_id=s.id
LEFT JOIN course_graphs g ON g.subject_id=s.id
LEFT JOIN course_graph_inputs i ON i.subject_id=s.id WHERE s.id=$1;
