-- name: CreateSubject :one
INSERT INTO subjects(id, name, created_at)
VALUES ($1, $2, $3)
RETURNING id, name;

-- name: ListSubjects :many
SELECT subject.id, subject.name,
       EXISTS (
           SELECT 1
           FROM competencies competency
           JOIN constituents constituent ON constituent.competency_id = competency.id
           JOIN outcomes outcome ON outcome.constituent_id = constituent.id
           JOIN taxonomies taxonomy ON taxonomy.id = outcome.taxonomy_id
           JOIN tasks task ON task.outcome_id = outcome.id
           WHERE competency.revision = subject.active_revision
             AND outcome.include_in_test IS TRUE
             AND taxonomy.code IN ('knowledge', 'understanding', 'application', 'analysis')
             AND outcome.importance BETWEEN 1 AND 5
             AND task.origin IN ('authored', 'ai_generated')
             AND task.question ~ '[^[:space:]]'
             AND task.voice_instruction ~ '[^[:space:]]'
             AND task.reference_answer ~ '[^[:space:]]'
       ) AS ready
FROM subjects subject
ORDER BY lower(subject.name), subject.id;
