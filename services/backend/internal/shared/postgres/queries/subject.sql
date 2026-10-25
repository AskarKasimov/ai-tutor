-- name: CreateSubject :one
INSERT INTO subjects(id, name, created_at)
VALUES ($1, $2, $3)
RETURNING id, name;

-- name: ListSubjects :many
SELECT subject.id, subject.name,
       EXISTS (
           SELECT 1 FROM course_graphs graph
           WHERE graph.subject_id = subject.id
             AND graph.source = 'materials'
             AND EXISTS (SELECT 1 FROM subject_materials material WHERE material.subject_id = subject.id)
       ) AS exam_ready,
       EXISTS (
           SELECT 1
           FROM learning_subject_state state
           JOIN learning_competencies competency ON competency.revision = state.active_revision
           JOIN learning_constituents constituent ON constituent.competency_id = competency.id
           JOIN learning_outcomes outcome ON outcome.constituent_id = constituent.id
           JOIN learning_taxonomies taxonomy ON taxonomy.id = outcome.taxonomy_id
           JOIN learning_tasks task ON task.outcome_id = outcome.id
           WHERE state.subject_id = subject.id
             AND outcome.include_in_test IS TRUE
             AND taxonomy.code IN ('knowledge', 'understanding', 'application', 'analysis')
             AND outcome.importance BETWEEN 1 AND 5
             AND task.origin IN ('authored', 'ai_generated')
             AND task.question ~ '[^[:space:]]'
             AND task.voice_instruction ~ '[^[:space:]]'
             AND task.reference_answer ~ '[^[:space:]]'
           GROUP BY competency.id
           HAVING count(DISTINCT outcome.id) >= 3
       ) AS learning_ready
FROM subjects subject
ORDER BY lower(subject.name), subject.id;
