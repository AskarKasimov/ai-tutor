-- +goose Up
CREATE TABLE material_kinds (
 code text PRIMARY KEY CHECK (length(trim(code)) > 0),
 label text NOT NULL CHECK (length(trim(label)) > 0),
 sort_order integer NOT NULL DEFAULT 0
);
INSERT INTO material_kinds(code,label,sort_order) VALUES ('lecture','Лекция',10),('lab','Лабораторная',20),('tutorial','Практика',30);
ALTER TABLE subject_materials ADD COLUMN week integer CHECK (week IS NULL OR week > 0), ADD COLUMN kind text REFERENCES material_kinds(code);

-- +goose Down
ALTER TABLE subject_materials DROP COLUMN kind, DROP COLUMN week;
DROP TABLE material_kinds;
