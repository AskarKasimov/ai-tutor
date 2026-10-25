-- name: CreateSubject :one
INSERT INTO subjects(id, name, created_at, mode)
VALUES ($1, $2, $3, 'exam')
RETURNING id, name;

-- name: CreateLearningSubject :one
INSERT INTO subjects(id, name, created_at, mode)
VALUES ($1, $2, $3, 'learn')
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

-- name: ListLearningSubjects :many
SELECT subject.id, subject.name,
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
       ) AS ready
FROM subjects subject
WHERE subject.mode = 'learn'
ORDER BY lower(subject.name), subject.id;
