-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertFile :exec
INSERT INTO file (id, name, state, storage_key, created_by)
VALUES (sqlc.arg(id), sqlc.arg(name), 'uploading', sqlc.arg(storage_key), sqlc.narg(created_by));

-- name: FinishFile :one
UPDATE file
SET state = 'temporary', size = sqlc.arg(size), sha256 = sqlc.arg(sha256), content_type = sqlc.arg(content_type)
WHERE id = sqlc.arg(id) AND state = 'uploading'
RETURNING id, name, content_type, size, sha256, state, storage_key, created_at, created_by;

-- name: GetFile :one
SELECT id, name, content_type, size, sha256, state, storage_key, created_at, created_by
FROM file WHERE id = sqlc.arg(id);

-- name: DeleteFile :exec
DELETE FROM file WHERE id = sqlc.arg(id);

-- name: MarkFileStored :exec
UPDATE file SET state = 'stored' WHERE id = sqlc.arg(id) AND state = 'temporary';

-- name: InsertAttachment :execrows
INSERT INTO attachment (id, file_id, type_name, record_id, created_by, entry_id)
VALUES (sqlc.arg(id), sqlc.arg(file_id), sqlc.arg(type_name), sqlc.arg(record_id), sqlc.narg(created_by), sqlc.narg(entry_id))
ON CONFLICT (file_id, type_name, record_id) DO NOTHING;

-- name: DeleteAttachment :execrows
DELETE FROM attachment
WHERE file_id = sqlc.arg(file_id) AND type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id);

-- name: AttachmentsOfFile :many
SELECT type_name, record_id, entry_id FROM attachment WHERE file_id = sqlc.arg(file_id) ORDER BY created_at;

-- name: FilesOfRecord :many
SELECT f.id, f.name, f.content_type, f.size, f.sha256, f.state, f.created_at, f.created_by,
       a.created_at AS attached_at, a.created_by AS attached_by, a.entry_id,
       coalesce(e.private, false)::boolean AS entry_private
FROM attachment a JOIN file f ON f.id = a.file_id
LEFT JOIN entry e ON e.id = a.entry_id
WHERE a.type_name = sqlc.arg(type_name) AND a.record_id = sqlc.arg(record_id)
ORDER BY a.created_at, f.id;

-- name: StaleFiles :many
-- Uploads that never finished and temporary files nobody attached, oldest first.
SELECT id, storage_key FROM file f
WHERE (f.state = 'uploading' AND f.created_at < sqlc.arg(uploading_before))
   OR (f.state = 'temporary' AND f.created_at < sqlc.arg(temporary_before)
       AND NOT EXISTS (SELECT 1 FROM attachment a WHERE a.file_id = f.id))
ORDER BY f.created_at
LIMIT 1000;
