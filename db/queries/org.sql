-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertOrgNode :one
INSERT INTO org_node (path, name, label, tenant)
VALUES (sqlc.arg(path)::ltree, sqlc.arg(name), sqlc.arg(label), sqlc.arg(tenant))
RETURNING path::text AS path, name, label, tenant, created_at, updated_at;

-- name: GetOrgNode :one
SELECT path::text AS path, name, label, tenant, created_at, updated_at
FROM org_node WHERE path = sqlc.arg(path)::ltree;

-- name: ListOrgNodes :many
SELECT path::text AS path, name, label, tenant, created_at, updated_at
FROM org_node ORDER BY path;

-- name: ListTenants :many
-- The tenants of the installation (PL44); none means the root is the only one.
SELECT path::text AS path, name, label FROM org_node WHERE tenant ORDER BY path;

-- name: OrgNodeExists :one
SELECT EXISTS (SELECT 1 FROM org_node WHERE path = sqlc.arg(path)::ltree);

-- name: CountOrgChildren :one
SELECT count(*) FROM org_node WHERE path <@ sqlc.arg(path)::ltree AND path <> sqlc.arg(path)::ltree;

-- name: DeleteOrgNode :exec
DELETE FROM org_node WHERE path = sqlc.arg(path)::ltree;

-- name: UpsertOrgSetting :exec
INSERT INTO org_setting (path, key, value, updated_by)
VALUES (sqlc.arg(path)::ltree, sqlc.arg(key), sqlc.arg(value), sqlc.arg(updated_by))
ON CONFLICT (path, key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by;

-- name: DeleteOrgSetting :execrows
DELETE FROM org_setting WHERE path = sqlc.arg(path)::ltree AND key = sqlc.arg(key);

-- name: OwnOrgSettings :many
SELECT key, value FROM org_setting WHERE path = sqlc.arg(path)::ltree ORDER BY key;

-- name: EffectiveOrgSettings :many
-- The nearest ancestor (or the node itself) that sets a key wins.
SELECT DISTINCT ON (key) key, value, path::text AS source
FROM org_setting
WHERE path @> sqlc.arg(path)::ltree
ORDER BY key, nlevel(path) DESC;
