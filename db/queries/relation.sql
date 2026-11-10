-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertRelation :one
INSERT INTO relation (id, kind, from_type, from_id, to_type, to_id, created_by)
VALUES (sqlc.arg(id), sqlc.arg(kind), sqlc.arg(from_type), sqlc.arg(from_id), sqlc.arg(to_type), sqlc.arg(to_id), sqlc.narg(created_by))
ON CONFLICT (kind, from_type, from_id, to_type, to_id) DO UPDATE SET kind = EXCLUDED.kind
RETURNING id, kind, from_type, from_id, to_type, to_id, created_at, created_by;

-- name: GetRelation :one
SELECT id, kind, from_type, from_id, to_type, to_id, created_at, created_by
FROM relation WHERE id = sqlc.arg(id);

-- name: DeleteRelation :execrows
DELETE FROM relation WHERE id = sqlc.arg(id);

-- name: ListRelationsOf :many
SELECT id, kind, from_type, from_id, to_type, to_id, created_at, created_by
FROM relation
WHERE ((from_type = sqlc.arg(type_name) AND from_id = sqlc.arg(record_id) AND sqlc.arg(outgoing)::boolean)
    OR (to_type = sqlc.arg(type_name) AND to_id = sqlc.arg(record_id) AND sqlc.arg(incoming)::boolean))
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
ORDER BY kind, created_at;

-- name: DeleteRelationsOfRecord :execrows
DELETE FROM relation
WHERE (from_type = sqlc.arg(type_name) AND from_id = sqlc.arg(record_id))
   OR (to_type = sqlc.arg(type_name) AND to_id = sqlc.arg(record_id));

-- name: CountRelationsTo :many
-- How many records link to each of the targets with one kind (SD18: "used at 7 tickets").
SELECT to_id, count(*)::bigint AS uses
FROM relation
WHERE kind = sqlc.arg(kind) AND to_type = sqlc.arg(to_type) AND to_id = ANY(sqlc.arg(to_ids)::uuid[])
GROUP BY to_id;
