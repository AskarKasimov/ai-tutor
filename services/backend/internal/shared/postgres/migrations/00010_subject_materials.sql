-- +goose Up
CREATE TABLE subject_materials (
    id text PRIMARY KEY,
    subject_id text NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    filename text NOT NULL,
    content_type text NOT NULL,
    file_size bigint NOT NULL DEFAULT 0,
    s3_raw_key text NOT NULL,
    s3_parsed_key text NOT NULL,
    parsed_text text NOT NULL DEFAULT '',
    char_count integer NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'parsed' CHECK (status IN ('pending', 'parsed', 'failed')),
    error_message text,
    created_at bigint NOT NULL,
    updated_at bigint NOT NULL
);

CREATE INDEX subject_materials_subject_idx ON subject_materials(subject_id);

-- +goose Down
DROP TABLE IF EXISTS subject_materials;
