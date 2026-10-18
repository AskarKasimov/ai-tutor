-- name: RequestCourseGraphBuild :one
-- The admin's "build graph" button: queue a build of the current materials
-- unless one is already queued or running. No row: inputs are not ready
-- (a material is still processing or failed, or the course has none) or a
-- build is already in progress.
INSERT INTO course_graph_jobs(subject_id, fingerprint, updated_at)
SELECT i.subject_id, i.fingerprint, sqlc.arg(now)::bigint FROM course_graph_inputs i WHERE i.subject_id = sqlc.arg(subject_id)
ON CONFLICT(subject_id) DO UPDATE SET fingerprint=excluded.fingerprint,
    status='pending', attempts=0, available_at=0, lease_until=0, lease_token='', error_message='', updated_at=excluded.updated_at
WHERE course_graph_jobs.status NOT IN ('pending', 'processing')
RETURNING subject_id;

-- name: ClaimCourseGraph :one
-- A requested build takes the materials as they are when it starts; it waits
-- while any material of the course is still being processed.
UPDATE course_graph_jobs j SET status='processing', fingerprint=i.fingerprint,
    attempts=j.attempts+CASE WHEN j.status='pending' THEN 1 ELSE 0 END,
    lease_token=sqlc.arg(token), lease_until=sqlc.arg(now)::bigint+60, updated_at=sqlc.arg(now)
FROM course_graph_inputs i
WHERE i.subject_id=j.subject_id AND j.subject_id=(SELECT c.subject_id FROM course_graph_jobs c
    WHERE ((c.status='pending' AND c.available_at<=sqlc.arg(now))
       OR (c.status='processing' AND c.lease_until<=sqlc.arg(now)))
      AND EXISTS(SELECT 1 FROM course_graph_inputs r WHERE r.subject_id=c.subject_id)
    ORDER BY c.updated_at FOR UPDATE OF c SKIP LOCKED LIMIT 1)
RETURNING j.*;

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
SELECT s.id, coalesce(j.status,'not_requested')::text AS status,
       coalesce(j.attempts,0)::integer AS attempts, coalesce(j.error_message,'')::text AS error_message,
       coalesce(g.graph,'null'::jsonb)::jsonb AS graph,
       coalesce(g.updated_at,0)::bigint AS updated_at,
       (g.fingerprint IS NOT NULL AND g.fingerprint IS DISTINCT FROM i.fingerprint)::boolean AS stale,
       (i.subject_id IS NOT NULL)::boolean AS inputs_ready
FROM subjects s LEFT JOIN course_graph_jobs j ON j.subject_id=s.id
LEFT JOIN course_graphs g ON g.subject_id=s.id
LEFT JOIN course_graph_inputs i ON i.subject_id=s.id WHERE s.id=$1;

-- name: ReadPublishedCourseGraph :one
-- The graph Geek examines by. It stays published while materials change
-- until the next build, but a course without materials has none.
SELECT g.graph FROM course_graphs g
WHERE g.subject_id=$1 AND EXISTS(SELECT 1 FROM subject_materials m WHERE m.subject_id=g.subject_id);

-- name: ListPublishedCourseGraphs :many
-- Every graph Geek examines by, under the condition of ReadPublishedCourseGraph;
-- the newest first, so their questions are voiced first.
SELECT g.graph FROM course_graphs g
WHERE EXISTS(SELECT 1 FROM subject_materials m WHERE m.subject_id=g.subject_id)
ORDER BY g.updated_at DESC, g.subject_id;
