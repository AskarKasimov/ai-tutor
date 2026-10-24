-- name: CreateSubject :one
INSERT INTO subjects(id, name, created_at)
VALUES ($1, $2, $3)
RETURNING id, name;

-- name: ListSubjects :many
SELECT subject.id, subject.name,
       -- A course is ready once Geek can read its own published graph, under
       -- the same condition as ReadPublishedCourseGraph.
       EXISTS (
           SELECT 1
           FROM course_graphs graph
           WHERE graph.subject_id = subject.id
             AND (graph.source = 'competency_map'
                  OR EXISTS (SELECT 1 FROM subject_materials material WHERE material.subject_id = subject.id))
       ) AS ready
FROM subjects subject
WHERE subject.mode = 'exam'
ORDER BY lower(subject.name), subject.id;
