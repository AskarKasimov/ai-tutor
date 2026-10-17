-- name: ListSubjectMaterials :many
SELECT *
FROM subject_materials WHERE subject_id=$1 ORDER BY created_at DESC;

-- name: GetSubjectMaterialByID :one
SELECT *
FROM subject_materials WHERE id=$1;

-- name: InsertSubjectMaterial :one
INSERT INTO subject_materials (
    id, subject_id, filename, content_type, file_size, s3_raw_key, s3_parsed_key,
    parsed_text, char_count, status, error_message, created_at, updated_at, week, kind
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
RETURNING *;

-- name: DeleteSubjectMaterial :exec
DELETE FROM subject_materials WHERE id=$1 AND subject_id=$2;

-- name: LockSubjectMaterial :one
SELECT id FROM subject_materials WHERE id=$1 FOR UPDATE;

-- name: ListMaterialKinds :many
SELECT code,label FROM material_kinds ORDER BY sort_order,code;

-- name: UpdateMaterialMetadata :one
UPDATE subject_materials SET week=coalesce(sqlc.narg(week)::integer,week), kind=coalesce(sqlc.narg(kind)::text,kind)
WHERE id=sqlc.arg(id) AND subject_id=sqlc.arg(subject_id) RETURNING *;

-- name: ReplaceSubjectMaterial :one
UPDATE subject_materials SET filename=sqlc.arg(filename),content_type=sqlc.arg(content_type),file_size=sqlc.arg(file_size),s3_raw_key=sqlc.arg(s3_raw_key),s3_parsed_key=sqlc.arg(s3_parsed_key),s3_ocr_key='',ocr_report='{}'::jsonb,parsed_text='',char_count=0,status='pending',error_message=NULL,updated_at=sqlc.arg(updated_at)
WHERE id=sqlc.arg(id) AND subject_id=sqlc.arg(subject_id) RETURNING *;

-- name: DeleteMaterialOCRJob :exec
DELETE FROM material_ocr_jobs WHERE material_id=$1;

-- name: DeleteMaterialCleaningJob :exec
DELETE FROM material_cleaning_jobs WHERE material_id=$1;
