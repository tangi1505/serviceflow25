-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertActor :exec
INSERT INTO actor (type_name, record_id, role, target_type, target_id, created_by)
VALUES (sqlc.arg(type_name), sqlc.arg(record_id), sqlc.arg(role), sqlc.arg(target_type), sqlc.arg(target_id), sqlc.narg(created_by));

-- name: DeleteActorsOfRole :exec
DELETE FROM actor WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id) AND role = sqlc.arg(role);

-- name: DeleteActorsOfRecord :exec
DELETE FROM actor WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id);
