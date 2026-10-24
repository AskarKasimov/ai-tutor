-- +goose Up
ALTER TABLE subjects DROP CONSTRAINT subjects_active_revision_fkey;
ALTER TABLE competency_map_state RENAME TO learning_map_state;
ALTER TABLE competency_map_imports RENAME TO learning_map_imports;
ALTER TABLE competency_map_source_rows RENAME TO learning_map_source_rows;
ALTER TABLE competencies RENAME TO learning_competencies;
ALTER TABLE constituents RENAME TO learning_constituents;
ALTER TABLE taxonomies RENAME TO learning_taxonomies;
ALTER TABLE ald_levels RENAME TO learning_ald_levels;
ALTER TABLE topic_levels RENAME TO learning_topic_levels;
ALTER TABLE outcomes RENAME TO learning_outcomes;
ALTER TABLE curriculum_sections RENAME TO learning_curriculum_sections;
ALTER TABLE curriculum_competencies RENAME TO learning_curriculum_competencies;
ALTER TABLE constituent_sections RENAME TO learning_constituent_sections;
ALTER TABLE constituent_section_competencies RENAME TO learning_constituent_section_competencies;
ALTER TABLE generation_runs RENAME TO learning_generation_runs;
ALTER TABLE tasks RENAME TO learning_tasks;
ALTER TABLE outcome_source_rows RENAME TO learning_outcome_source_rows;
ALTER TABLE material_chunks RENAME TO learning_material_chunks;
ALTER TABLE material_chunk_outcomes RENAME TO learning_material_chunk_outcomes;
ALTER TABLE generation_run_examples RENAME TO learning_generation_run_examples;
ALTER TABLE generation_run_chunks RENAME TO learning_generation_run_chunks;
ALTER TABLE material_embeddings RENAME TO learning_material_embeddings;
ALTER TABLE task_assignments RENAME TO learning_task_assignments;
ALTER TABLE assignment_responses RENAME TO learning_assignment_responses;
ALTER TABLE variants RENAME TO learning_variants;
ALTER TABLE variant_tasks RENAME TO learning_variant_tasks;
ALTER TABLE audio_assets RENAME TO learning_audio_assets;
ALTER TABLE diagnostic_sessions RENAME TO learning_diagnostic_sessions;
ALTER TABLE diagnostic_answers RENAME TO learning_diagnostic_answers;
ALTER TABLE training_sessions RENAME TO learning_training_sessions;
ALTER TABLE training_start_requests RENAME TO learning_training_start_requests;
ALTER TABLE training_targets RENAME TO learning_training_targets;
ALTER TABLE training_exercises RENAME TO learning_training_exercises;
ALTER TABLE training_attempts RENAME TO learning_training_attempts;
ALTER TABLE training_chats RENAME TO learning_training_chats;
ALTER TABLE training_chat_messages RENAME TO learning_training_chat_messages;
ALTER TABLE training_chat_turns RENAME TO learning_training_chat_turns;
ALTER TABLE learning_messages RENAME TO learning_plan_messages;
ALTER TABLE diagnostic_feedback RENAME TO learning_diagnostic_feedback;
ALTER VIEW constituent_curriculum_profiles RENAME TO learning_constituent_curriculum_profiles;

CREATE TABLE learning_subject_state (
    subject_id text PRIMARY KEY REFERENCES subjects(id) ON DELETE CASCADE,
    active_revision bigint REFERENCES learning_map_imports(revision) ON DELETE SET NULL
);
INSERT INTO learning_subject_state(subject_id, active_revision)
SELECT id, active_revision FROM subjects WHERE active_revision IS NOT NULL;
ALTER TABLE subjects DROP COLUMN active_revision;

-- +goose Down
ALTER VIEW learning_constituent_curriculum_profiles RENAME TO constituent_curriculum_profiles;
ALTER TABLE learning_diagnostic_feedback RENAME TO diagnostic_feedback;
ALTER TABLE learning_plan_messages RENAME TO learning_messages;
ALTER TABLE learning_training_chat_turns RENAME TO training_chat_turns;
ALTER TABLE learning_training_chat_messages RENAME TO training_chat_messages;
ALTER TABLE learning_training_chats RENAME TO training_chats;
ALTER TABLE learning_training_attempts RENAME TO training_attempts;
ALTER TABLE learning_training_exercises RENAME TO training_exercises;
ALTER TABLE learning_training_targets RENAME TO training_targets;
ALTER TABLE learning_training_start_requests RENAME TO training_start_requests;
ALTER TABLE learning_training_sessions RENAME TO training_sessions;
ALTER TABLE learning_diagnostic_answers RENAME TO diagnostic_answers;
ALTER TABLE learning_diagnostic_sessions RENAME TO diagnostic_sessions;
ALTER TABLE learning_audio_assets RENAME TO audio_assets;
ALTER TABLE learning_variant_tasks RENAME TO variant_tasks;
ALTER TABLE learning_variants RENAME TO variants;
ALTER TABLE learning_assignment_responses RENAME TO assignment_responses;
ALTER TABLE learning_task_assignments RENAME TO task_assignments;
ALTER TABLE learning_material_embeddings RENAME TO material_embeddings;
ALTER TABLE learning_generation_run_chunks RENAME TO generation_run_chunks;
ALTER TABLE learning_generation_run_examples RENAME TO generation_run_examples;
ALTER TABLE learning_material_chunk_outcomes RENAME TO material_chunk_outcomes;
ALTER TABLE learning_material_chunks RENAME TO material_chunks;
ALTER TABLE learning_outcome_source_rows RENAME TO outcome_source_rows;
ALTER TABLE learning_tasks RENAME TO tasks;
ALTER TABLE learning_generation_runs RENAME TO generation_runs;
ALTER TABLE learning_constituent_section_competencies RENAME TO constituent_section_competencies;
ALTER TABLE learning_constituent_sections RENAME TO constituent_sections;
ALTER TABLE learning_curriculum_competencies RENAME TO curriculum_competencies;
ALTER TABLE learning_curriculum_sections RENAME TO curriculum_sections;
ALTER TABLE learning_outcomes RENAME TO outcomes;
ALTER TABLE learning_topic_levels RENAME TO topic_levels;
ALTER TABLE learning_ald_levels RENAME TO ald_levels;
ALTER TABLE learning_taxonomies RENAME TO taxonomies;
ALTER TABLE learning_constituents RENAME TO constituents;
ALTER TABLE learning_competencies RENAME TO competencies;
ALTER TABLE learning_map_source_rows RENAME TO competency_map_source_rows;
ALTER TABLE learning_map_imports RENAME TO competency_map_imports;
ALTER TABLE learning_map_state RENAME TO competency_map_state;
ALTER TABLE subjects ADD COLUMN active_revision bigint;
ALTER TABLE subjects ADD CONSTRAINT subjects_active_revision_fkey
    FOREIGN KEY (active_revision) REFERENCES competency_map_imports(revision) ON DELETE SET NULL;
UPDATE subjects AS subject SET active_revision = state.active_revision
FROM learning_subject_state AS state WHERE state.subject_id = subject.id;
DROP TABLE learning_subject_state;
