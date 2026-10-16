-- name: CreateSubject :one
INSERT INTO subjects(id, name, created_at)
VALUES ($1, $2, $3)
RETURNING id, name;

-- name: ListSubjects :many
SELECT subject.id, subject.name,
       -- Interim until the course graph is stored: a course is ready once it
       -- has a parsed material.
       EXISTS (
           SELECT 1
           FROM subject_materials material
           WHERE material.subject_id = subject.id
             AND material.status = 'parsed'
       ) AS ready
FROM subjects subject
ORDER BY lower(subject.name), subject.id;
