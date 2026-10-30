-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertEntry :one
INSERT INTO entry (id, type_name, record_id, body, format, private, created_by)
VALUES (sqlc.arg(id), sqlc.arg(type_name), sqlc.arg(record_id), sqlc.arg(body), sqlc.arg(format), sqlc.arg(private), sqlc.narg(created_by))
RETURNING created_at;

-- name: GetEntry :one
SELECT id, body, format, private, created_at, created_by
FROM entry
WHERE id = sqlc.arg(id) AND type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id);

-- name: EntriesOfRecord :many
-- with_private false leaves the private entries out.
SELECT id, body, format, private, created_at, created_by
FROM entry
WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id)
  AND (sqlc.arg(with_private)::boolean OR NOT private)
ORDER BY created_at, id;

-- name: DeleteEntriesOfRecord :exec
DELETE FROM entry WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id);

-- name: AccountNames :many
SELECT id, display_name FROM account WHERE id = ANY(sqlc.arg(ids)::uuid[]);
