-- +goose Up
-- A course graph is built either from the course materials or from an
-- imported competency map (decision of 19.10.2026); the last build replaces
-- the graph whichever its source.
CREATE TABLE course_graph_maps (
    subject_id text PRIMARY KEY REFERENCES subjects(id) ON DELETE CASCADE,
    filename text NOT NULL,
    sheet text NOT NULL,
    fingerprint text NOT NULL,
    graph jsonb NOT NULL,
    uploaded_at bigint NOT NULL
);

ALTER TABLE course_graph_jobs ADD COLUMN source text NOT NULL DEFAULT 'materials'
    CHECK (source IN ('materials', 'competency_map'));
ALTER TABLE course_graphs ADD COLUMN source text NOT NULL DEFAULT 'materials'
    CHECK (source IN ('materials', 'competency_map'));

-- The inputs a build of either source is checked against: the materials are
-- ready under course_graph_inputs, a map as soon as it is uploaded.
CREATE VIEW course_graph_job_inputs AS
SELECT subject_id, 'materials'::text AS source, fingerprint FROM course_graph_inputs
UNION ALL
SELECT subject_id, 'competency_map'::text AS source, fingerprint FROM course_graph_maps;

-- +goose Down
DROP VIEW course_graph_job_inputs;
ALTER TABLE course_graphs DROP COLUMN source;
ALTER TABLE course_graph_jobs DROP COLUMN source;
DROP TABLE course_graph_maps;
