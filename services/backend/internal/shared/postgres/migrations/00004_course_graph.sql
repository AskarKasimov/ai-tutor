-- +goose Up
CREATE VIEW course_graph_inputs AS
SELECT m.subject_id,
       md5(string_agg(m.id || ':' || coalesce(c.s3_report_key, ''), ',' ORDER BY m.id)) AS fingerprint
FROM subject_materials m
LEFT JOIN material_cleaning_jobs c ON c.material_id = m.id
GROUP BY m.subject_id
HAVING bool_and(m.status = 'parsed' AND coalesce(c.status = 'ready', false));

CREATE TABLE course_graph_jobs (
    subject_id text PRIMARY KEY REFERENCES subjects(id) ON DELETE CASCADE,
    fingerprint text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','ready','failed')),
    attempts integer NOT NULL DEFAULT 0,
    available_at bigint NOT NULL DEFAULT 0,
    lease_until bigint NOT NULL DEFAULT 0,
    lease_token text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    updated_at bigint NOT NULL
);
CREATE TABLE course_graphs (
    subject_id text PRIMARY KEY REFERENCES subjects(id) ON DELETE CASCADE,
    fingerprint text NOT NULL,
    graph jsonb NOT NULL,
    updated_at bigint NOT NULL
);

-- +goose Down
DROP TABLE course_graphs;
DROP TABLE course_graph_jobs;
DROP VIEW course_graph_inputs;
