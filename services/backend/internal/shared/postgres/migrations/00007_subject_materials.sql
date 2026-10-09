-- +goose Up
ALTER TABLE material_chunks ADD COLUMN subject_id text NOT NULL DEFAULT 'subject:intro-to-ml';

UPDATE material_chunks chunk
SET subject_id = subject.id
FROM material_chunk_outcomes link
JOIN outcomes outcome ON outcome.id = link.outcome_id
JOIN constituents constituent ON constituent.id = outcome.constituent_id
JOIN competencies competency ON competency.id = constituent.competency_id
JOIN subjects subject ON subject.active_revision = competency.revision
WHERE link.chunk_id = chunk.id;

ALTER TABLE material_chunks ALTER COLUMN subject_id DROP DEFAULT;
ALTER TABLE material_chunks ADD CONSTRAINT material_chunks_subject_fkey
    FOREIGN KEY (subject_id) REFERENCES subjects(id) ON DELETE CASCADE;
CREATE INDEX material_chunks_subject_name ON material_chunks(subject_id, material_name);

-- +goose Down
DROP INDEX material_chunks_subject_name;
ALTER TABLE material_chunks DROP CONSTRAINT material_chunks_subject_fkey;
ALTER TABLE material_chunks DROP COLUMN subject_id;
