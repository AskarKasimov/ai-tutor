-- name: ListSubjectMaterials :many
SELECT
    id,
    subject_id,
    filename,
    content_type,
    file_size,
    s3_raw_key,
    s3_parsed_key,
    parsed_text,
    char_count,
    status,
    error_message,
    created_at,
    updated_at
FROM subject_materials
WHERE subject_id = $1
ORDER BY created_at DESC;

-- name: GetSubjectMaterialByID :one
SELECT
    id,
    subject_id,
    filename,
    content_type,
    file_size,
    s3_raw_key,
    s3_parsed_key,
    parsed_text,
    char_count,
    status,
    error_message,
    created_at,
    updated_at
FROM subject_materials
WHERE id = $1;

-- name: InsertSubjectMaterial :one
INSERT INTO subject_materials (
    id,
    subject_id,
    filename,
    content_type,
    file_size,
    s3_raw_key,
    s3_parsed_key,
    parsed_text,
    char_count,
    status,
    error_message,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING
    id,
    subject_id,
    filename,
    content_type,
    file_size,
    s3_raw_key,
    s3_parsed_key,
    parsed_text,
    char_count,
    status,
    error_message,
    created_at,
    updated_at;

-- name: DeleteSubjectMaterial :exec
DELETE FROM subject_materials
WHERE id = $1 AND subject_id = $2;

-- name: LockSubjectMaterial :one
SELECT id FROM subject_materials WHERE id = $1 FOR UPDATE;
