-- +goose Up
DROP VIEW course_graph_inputs;
CREATE VIEW course_graph_inputs AS
SELECT m.subject_id,
       md5(string_agg(
           jsonb_build_array(m.id, m.s3_raw_key, coalesce(c.s3_report_key, ''), m.week, m.kind, k.label, m.filename)::text,
           ',' ORDER BY m.id
       )) AS fingerprint
FROM subject_materials m
LEFT JOIN material_cleaning_jobs c ON c.material_id = m.id
LEFT JOIN material_kinds k ON k.code = m.kind
GROUP BY m.subject_id
HAVING bool_and(m.status = 'parsed' AND coalesce(c.status = 'ready', false)
                AND m.week IS NOT NULL AND m.kind IS NOT NULL AND k.code IS NOT NULL);

-- +goose Down
DROP VIEW course_graph_inputs;
CREATE VIEW course_graph_inputs AS
SELECT m.subject_id,
       md5(string_agg(m.id || ':' || coalesce(c.s3_report_key, ''), ',' ORDER BY m.id)) AS fingerprint
FROM subject_materials m
LEFT JOIN material_cleaning_jobs c ON c.material_id = m.id
GROUP BY m.subject_id
HAVING bool_and(m.status = 'parsed' AND coalesce(c.status = 'ready', false));
