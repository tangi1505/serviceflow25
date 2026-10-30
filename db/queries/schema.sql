-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertSchemaChange :exec
INSERT INTO schema_change (id, type_name, change, statement) VALUES ($1, $2, $3, $4);

-- name: ListSchemaChanges :many
SELECT id, type_name, change, statement, applied_at FROM schema_change
WHERE type_name = $1 ORDER BY applied_at, id;

-- name: TableColumns :many
SELECT column_name::text AS name, data_type::text AS data_type, udt_name::text AS udt_name
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = $1
ORDER BY ordinal_position;
