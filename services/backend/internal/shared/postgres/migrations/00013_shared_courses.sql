-- +goose Up
-- Keep the old label only as rollback evidence. The application no longer
-- classifies courses by mode; each module owns its own resource tables.
CREATE TABLE subject_mode_rollback (
    subject_id text PRIMARY KEY REFERENCES subjects(id) ON DELETE CASCADE,
    mode text NOT NULL CHECK (mode IN ('exam', 'learn'))
);
INSERT INTO subject_mode_rollback(subject_id, mode)
SELECT id, mode FROM subjects;
ALTER TABLE subjects DROP CONSTRAINT subjects_mode_check;
ALTER TABLE subjects DROP COLUMN mode;

-- +goose Down
ALTER TABLE subjects ADD COLUMN mode text NOT NULL DEFAULT 'exam'
    CHECK (mode IN ('exam', 'learn'));
UPDATE subjects subject SET mode = rollback.mode
FROM subject_mode_rollback rollback WHERE rollback.subject_id = subject.id;
DROP TABLE subject_mode_rollback;
