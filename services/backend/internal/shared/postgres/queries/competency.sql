-- name: LockSubjectForImport :one
SELECT id FROM subjects WHERE id = $1 FOR UPDATE;

-- name: LockCompetencyMapRevision :one
SELECT revision FROM competency_map_state WHERE singleton = true FOR UPDATE;

-- name: LockSubjectForTaskgen :one
SELECT active_revision FROM subjects WHERE id = $1 FOR SHARE;

-- name: DeleteCompetencyMapImportsForSubject :exec
DELETE FROM competency_map_imports WHERE subject_id = $1;

-- name: InsertCompetencyMapImport :exec
INSERT INTO competency_map_imports(
    revision, subject_id, imported_at, imported_by, competency_count, constituent_count,
    outcome_count, task_count, source_format, source_headers, unparsed_task_cell_count
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: UpdateSubjectActiveRevision :exec
UPDATE subjects SET active_revision = $2 WHERE id = $1;

-- name: InsertCompetency :exec
INSERT INTO competencies(id, name, revision) VALUES ($1, $2, $3);

-- name: InsertConstituent :exec
INSERT INTO constituents(id, competency_id, name, topic_level_id)
VALUES ($1, $2, $3, NULLIF(sqlc.arg(topic_level_id)::text, ''));

-- name: InsertOutcome :exec
INSERT INTO outcomes(
    id, constituent_id, name, include_in_test, taxonomy_id,
    ald_level_id, importance, educational_content
) VALUES (
    $1, $2, $3, $4,
    (SELECT id FROM taxonomies WHERE code = NULLIF(sqlc.arg(taxonomy_code)::text, '')),
    (SELECT id FROM ald_levels WHERE code = NULLIF(sqlc.arg(ald_level_code)::text, '')),
    NULLIF(sqlc.arg(importance)::smallint, 0), NULLIF(sqlc.arg(educational_content)::text, '')
);

-- name: InsertCurriculumSection :exec
INSERT INTO curriculum_sections(id, revision, code, title) VALUES ($1, $2, $3, $4);

-- name: InsertCurriculumCompetency :exec
INSERT INTO curriculum_competencies(id, revision, code) VALUES ($1, $2, $3);

-- name: InsertConstituentSection :exec
INSERT INTO constituent_sections(id, revision, constituent_id, section_id)
VALUES ($1, $2, $3, $4);

-- name: InsertConstituentSectionCompetency :exec
INSERT INTO constituent_section_competencies(revision, constituent_section_id, curriculum_competency_id)
VALUES ($1, $2, $3);

-- name: InsertTask :exec
INSERT INTO tasks(
    id, outcome_id, question, criteria, source_row, source_column, options,
    voice_instruction, reference_answer, origin, source_revision,
    source_row_index, source_column_index, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, 'authored',
    NULLIF(sqlc.arg(source_revision)::bigint, 0),
    NULLIF(sqlc.arg(source_row_index)::integer, 0),
    NULLIF(sqlc.arg(source_column_index)::integer, 0),
    sqlc.arg(created_at)::bigint
);

-- name: InsertCompetencyMapSourceRow :exec
INSERT INTO competency_map_source_rows(revision, row_index, source_line, cells)
VALUES ($1, $2, $3, $4);

-- name: InsertOutcomeSourceRow :exec
INSERT INTO outcome_source_rows(outcome_id, source_revision, source_row_index)
VALUES ($1, $2, $3);

-- name: UpdateCompetencyMapRevision :exec
UPDATE competency_map_state SET revision = $1 WHERE singleton = true;

-- name: ReadSubjectCompetencyMap :many
SELECT COALESCE(subject.active_revision, 0)::bigint AS revision, import.imported_at,
       competency.id AS competency_id, competency.name AS competency_name,
       constituent.id AS constituent_id, constituent.name AS constituent_name,
       topic.code AS topic_level_code,
       COALESCE(curriculum_profile.curriculum_sections, '[]')::text AS curriculum_sections,
       outcome.id AS outcome_id, outcome.name AS outcome_name,
       outcome.include_in_test, taxonomy.code AS taxonomy_code,
       ald.code AS ald_level_code, outcome.importance, outcome.educational_content,
       task.id AS task_id, task.question, task.options, task.voice_instruction, task.origin
FROM subjects AS subject
LEFT JOIN competency_map_imports AS import ON import.revision = subject.active_revision
LEFT JOIN competencies AS competency ON competency.revision = subject.active_revision
LEFT JOIN constituents AS constituent ON constituent.competency_id = competency.id
LEFT JOIN topic_levels AS topic ON topic.id = constituent.topic_level_id
LEFT JOIN constituent_curriculum_profiles AS curriculum_profile ON curriculum_profile.constituent_id = constituent.id
LEFT JOIN outcomes AS outcome ON outcome.constituent_id = constituent.id
LEFT JOIN taxonomies AS taxonomy ON taxonomy.id = outcome.taxonomy_id
LEFT JOIN ald_levels AS ald ON ald.id = outcome.ald_level_id
LEFT JOIN tasks AS task ON task.outcome_id = outcome.id
WHERE subject.id = $1
ORDER BY competency.name, competency.id, constituent.name, constituent.id,
         outcome.name, outcome.id, task.created_at, task.id;

-- name: GetTaskProfile :one
SELECT
    task.id AS task_id, task.question, task.options, task.voice_instruction, task.audio_asset_id,
    task.reference_answer, task.criteria, task.origin, task.created_at,
    outcome.id AS outcome_id, outcome.name AS outcome_name,
    outcome.include_in_test, taxonomy.code AS taxonomy_code,
    ald.code AS ald_level_code, outcome.importance, outcome.educational_content,
    constituent.id AS constituent_id, constituent.name AS constituent_name,
    topic.code AS topic_level_code,
    competency.id AS competency_id, competency.name AS competency_name,
    COALESCE(curriculum_profile.curriculum_sections, '[]')::text AS curriculum_sections
FROM tasks AS task
JOIN outcomes AS outcome ON outcome.id = task.outcome_id
JOIN constituents AS constituent ON constituent.id = outcome.constituent_id
JOIN constituent_curriculum_profiles AS curriculum_profile ON curriculum_profile.constituent_id = constituent.id
JOIN competencies AS competency ON competency.id = constituent.competency_id
JOIN subjects AS subject ON subject.id = sqlc.arg(subject_id)::text AND subject.active_revision = competency.revision
LEFT JOIN taxonomies AS taxonomy ON taxonomy.id = outcome.taxonomy_id
LEFT JOIN ald_levels AS ald ON ald.id = outcome.ald_level_id
LEFT JOIN topic_levels AS topic ON topic.id = constituent.topic_level_id
WHERE task.id = sqlc.arg(task_id)::text;

-- name: SearchTaskProfiles :many
SELECT task.id, task.question, task.options, task.voice_instruction,
       task.origin, outcome.id AS outcome_id, outcome.name AS outcome_name,
       competency.name AS competency_name, constituent.name AS constituent_name,
       taxonomy.code AS taxonomy_code, ald.code AS ald_level_code,
       topic.code AS topic_level_code, outcome.importance, outcome.include_in_test
FROM tasks AS task
JOIN outcomes AS outcome ON outcome.id = task.outcome_id
JOIN constituents AS constituent ON constituent.id = outcome.constituent_id
JOIN competencies AS competency ON competency.id = constituent.competency_id
JOIN subjects AS subject ON subject.id = sqlc.arg(subject_id)::text AND subject.active_revision = competency.revision
LEFT JOIN taxonomies AS taxonomy ON taxonomy.id = outcome.taxonomy_id
LEFT JOIN ald_levels AS ald ON ald.id = outcome.ald_level_id
LEFT JOIN topic_levels AS topic ON topic.id = constituent.topic_level_id
WHERE (sqlc.arg(outcome_id)::text = '' OR outcome.id = sqlc.arg(outcome_id)::text)
  AND (sqlc.arg(competency_id)::text = '' OR competency.id = sqlc.arg(competency_id)::text)
  AND (sqlc.arg(constituent_id)::text = '' OR constituent.id = sqlc.arg(constituent_id)::text)
  AND (sqlc.arg(taxonomy_code)::text = '' OR taxonomy.code = sqlc.arg(taxonomy_code)::text)
  AND (sqlc.arg(ald_level_code)::text = '' OR ald.code = sqlc.arg(ald_level_code)::text)
  AND (sqlc.arg(topic_level_code)::text = '' OR topic.code = sqlc.arg(topic_level_code)::text)
  AND (sqlc.arg(importance)::smallint = 0 OR outcome.importance = sqlc.arg(importance)::smallint)
  AND (sqlc.arg(importance_min)::smallint = 0 OR outcome.importance >= sqlc.arg(importance_min)::smallint)
  AND (sqlc.arg(importance_max)::smallint = 0 OR outcome.importance <= sqlc.arg(importance_max)::smallint)
  AND (sqlc.arg(include_in_test)::integer < 0 OR outcome.include_in_test = (sqlc.arg(include_in_test)::integer = 1))
  AND (sqlc.arg(origin)::text = '' OR task.origin = sqlc.arg(origin)::text)
  AND ((sqlc.arg(section_code)::text = '' AND sqlc.arg(curriculum_competency_code)::text = '') OR EXISTS (
      SELECT 1 FROM constituent_sections AS link
      JOIN curriculum_sections AS section ON section.id = link.section_id
      LEFT JOIN constituent_section_competencies AS mapped ON mapped.constituent_section_id = link.id
      LEFT JOIN curriculum_competencies AS curriculum ON curriculum.id = mapped.curriculum_competency_id
      WHERE link.constituent_id = constituent.id
        AND (sqlc.arg(section_code)::text = '' OR section.code = sqlc.arg(section_code)::text)
        AND (sqlc.arg(curriculum_competency_code)::text = '' OR curriculum.code = sqlc.arg(curriculum_competency_code)::text)
  ))
ORDER BY task.created_at, task.id
LIMIT sqlc.arg(task_limit)::integer;

-- name: InsertGenerationRun :execrows
INSERT INTO generation_runs(id, outcome_id, map_revision, model, prompt_version, request_key, requested_profile, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (outcome_id, request_key) DO NOTHING;

-- name: GetGeneratedTaskByKey :one
SELECT task.id, task.outcome_id, task.question, task.options, task.voice_instruction,
       task.reference_answer, task.criteria, task.origin
FROM generation_runs AS run
JOIN tasks AS task ON task.generation_run_id = run.id
JOIN outcomes AS outcome ON outcome.id = run.outcome_id
JOIN constituents AS constituent ON constituent.id = outcome.constituent_id
JOIN competencies AS competency ON competency.id = constituent.competency_id
JOIN subjects AS subject ON subject.id = sqlc.arg(subject_id)::text AND subject.active_revision = competency.revision
WHERE run.outcome_id = sqlc.arg(outcome_id)::text AND run.request_key = sqlc.arg(request_key)::text;

-- name: InsertGenerationRunExample :exec
INSERT INTO generation_run_examples(generation_run_id, task_id) VALUES ($1, $2);

-- name: InsertGenerationRunChunk :exec
INSERT INTO generation_run_chunks(generation_run_id, chunk_id) VALUES ($1, $2);

-- name: InsertGeneratedTask :exec
INSERT INTO tasks(id, outcome_id, question, options, voice_instruction, reference_answer, criteria,
                  origin, generation_run_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'ai_generated', $8, $9);

-- name: GetOutcomeForGeneration :one
SELECT subject.active_revision::bigint AS revision, outcome.id, outcome.name, outcome.include_in_test, taxonomy.code AS taxonomy_code,
       ald.code AS ald_level_code, outcome.importance, outcome.educational_content,
       constituent.name AS constituent_name, topic.code AS topic_level_code,
       competency.name AS competency_name,
       COALESCE(curriculum_profile.curriculum_sections, '[]')::text AS curriculum_sections
FROM outcomes AS outcome
JOIN constituents AS constituent ON constituent.id = outcome.constituent_id
JOIN constituent_curriculum_profiles AS curriculum_profile ON curriculum_profile.constituent_id = constituent.id
JOIN competencies AS competency ON competency.id = constituent.competency_id
JOIN subjects AS subject ON subject.id = sqlc.arg(subject_id)::text AND subject.active_revision = competency.revision
LEFT JOIN taxonomies AS taxonomy ON taxonomy.id = outcome.taxonomy_id
LEFT JOIN ald_levels AS ald ON ald.id = outcome.ald_level_id
LEFT JOIN topic_levels AS topic ON topic.id = constituent.topic_level_id
WHERE outcome.id = sqlc.arg(outcome_id)::text;

-- name: ListGenerationExamples :many
SELECT task.id, task.question, task.options, task.voice_instruction, task.reference_answer, task.criteria
FROM tasks AS task
WHERE task.outcome_id = $1
ORDER BY CASE WHEN task.origin = 'authored' THEN 0 ELSE 1 END, task.created_at DESC, task.id
LIMIT $2;

-- name: InsertMaterialChunk :exec
INSERT INTO material_chunks(id, subject_id, material_name, ordinal, content, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: DeleteMaterialLinksBySubjectAndName :exec
DELETE FROM material_chunk_outcomes
WHERE chunk_id IN (SELECT id FROM material_chunks WHERE subject_id = $1 AND material_name = $2);

-- name: LinkMaterialChunkToOutcome :exec
INSERT INTO material_chunk_outcomes(chunk_id, outcome_id) VALUES ($1, $2);

-- name: CountExistingOutcomesForSubject :one
SELECT count(*)::integer FROM outcomes outcome
JOIN constituents constituent ON constituent.id = outcome.constituent_id
JOIN competencies competency ON competency.id = constituent.competency_id
JOIN subjects subject ON subject.id = sqlc.arg(subject_id)::text AND subject.active_revision = competency.revision
WHERE outcome.id = ANY(sqlc.arg(outcome_ids)::text[]);

-- name: SearchMaterialChunks :many
SELECT chunk.id, chunk.material_name, chunk.ordinal, chunk.content,
       ts_rank_cd(chunk.search_vector, to_tsquery('russian', replace(plainto_tsquery('russian', sqlc.arg(search_query)::text)::text, ' & ', ' | '))) AS relevance
FROM material_chunks AS chunk
JOIN material_chunk_outcomes AS link ON link.chunk_id = chunk.id
WHERE link.outcome_id = sqlc.arg(outcome_id)::text AND chunk.subject_id = sqlc.arg(subject_id)::text
  AND chunk.search_vector @@ to_tsquery('russian', replace(plainto_tsquery('russian', sqlc.arg(search_query)::text)::text, ' & ', ' | '))
ORDER BY relevance DESC, chunk.id
LIMIT sqlc.arg(chunk_limit)::integer;
