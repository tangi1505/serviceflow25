-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: UpsertPackage :exec
INSERT INTO package (name, version, label, spec)
VALUES ($1, $2, $3, $4)
ON CONFLICT (name) DO UPDATE
SET version = EXCLUDED.version, label = EXCLUDED.label, spec = EXCLUDED.spec, updated_at = now();

-- name: GetPackage :one
SELECT name, version, label, spec, installed_at, updated_at FROM package WHERE name = $1;

-- name: ListPackages :many
SELECT name, version, label, spec, installed_at, updated_at FROM package ORDER BY name;

-- name: UpsertDefinition :one
INSERT INTO definition (id, package, kind, name, layer, spec)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (package, kind, name, layer) DO UPDATE
SET spec = EXCLUDED.spec, version = definition.version + 1, updated_at = now()
RETURNING id, version;

-- name: GetDefinition :one
SELECT id, spec, version FROM definition
WHERE package = $1 AND kind = $2 AND name = $3 AND layer = $4;

-- name: InsertDefinition :execrows
-- The first definition of a layer (PL43); a concurrent first write inserts nothing.
INSERT INTO definition (id, package, kind, name, layer, spec)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (package, kind, name, layer) DO NOTHING;

-- name: UpdateDefinitionVersioned :execrows
-- A definition of a layer changed only if it still has the version seen (PL43).
UPDATE definition SET spec = $5, version = version + 1, updated_at = now()
WHERE package = $1 AND kind = $2 AND name = $3 AND layer = $4 AND version = sqlc.arg(version);

-- name: ListLayerDefinitions :many
SELECT id, package, kind, name, layer, spec, version, created_at, updated_at
FROM definition
WHERE kind = $1 AND layer = $2
ORDER BY package, name;

-- name: ListDefinitions :many
SELECT id, package, kind, name, layer, spec, version, created_at, updated_at
FROM definition
WHERE kind = $1
ORDER BY package, name, layer;

-- name: ListPackageDefinitions :many
SELECT id, package, kind, name, layer, spec, version, created_at, updated_at
FROM definition
WHERE package = $1
ORDER BY kind, name, layer;

-- name: DeleteDefinitionsNotIn :exec
-- Removes base-layer definitions of a package that a new version no longer ships.
DELETE FROM definition
WHERE package = $1 AND layer = 'base' AND NOT (kind || ':' || name = ANY(sqlc.arg(keep)::text[]));
