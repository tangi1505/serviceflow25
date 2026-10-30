-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: GetRoleByName :one
SELECT id, name, label, description, created_at, updated_at FROM role WHERE name = $1;

-- name: ListRoles :many
SELECT id, name, label, description, created_at, updated_at FROM role ORDER BY name;

-- name: InsertRole :one
INSERT INTO role (id, name, label, description) VALUES ($1, $2, $3, $4)
RETURNING id, name, label, description, created_at, updated_at;

-- name: DeleteRole :execrows
DELETE FROM role WHERE id = $1 AND name <> 'admin';

-- name: ListPermissions :many
SELECT id, role_id, type_name, action, scope, condition, fields FROM permission WHERE role_id = $1 ORDER BY type_name, action;

-- name: UpsertPermission :one
INSERT INTO permission (id, role_id, type_name, action, scope, condition, fields)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (role_id, type_name, action, scope) DO UPDATE SET condition = EXCLUDED.condition, fields = EXCLUDED.fields
RETURNING id, role_id, type_name, action, scope, condition, fields;

-- name: DeletePermission :execrows
-- Without a scope every line of the type and action goes (UI4: a role may hold several).
DELETE FROM permission
WHERE role_id = sqlc.arg(role_id) AND type_name = sqlc.arg(type_name) AND action = sqlc.arg(action)
  AND (sqlc.narg(scope)::text IS NULL OR scope = sqlc.narg(scope)::text);

-- name: InsertAssignment :one
INSERT INTO role_assignment (id, role_id, person_id, group_id, org_path, created_by)
VALUES (sqlc.arg(id), sqlc.arg(role_id), sqlc.arg(person_id), sqlc.arg(group_id), sqlc.arg(org_path)::ltree, sqlc.arg(created_by))
RETURNING id, role_id, person_id, group_id, org_path::text AS org_path, created_at;

-- name: ListAssignments :many
SELECT a.id, a.role_id, r.name AS role_name, a.person_id, a.group_id, a.org_path::text AS org_path, a.created_at
FROM role_assignment a JOIN role r ON r.id = a.role_id
ORDER BY r.name, a.org_path;

-- name: DeleteAssignment :execrows
DELETE FROM role_assignment WHERE id = $1;

-- name: GrantsForPerson :many
-- Every permission that reaches a person: through assignments to the person and to the groups
-- the person is a member of (the caller resolves the groups through internal/query, E34).
SELECT p.type_name, p.action, p.scope, p.condition, p.fields, a.org_path::text AS org_path, r.name AS role_name
FROM role_assignment a
JOIN role r ON r.id = a.role_id
JOIN permission p ON p.role_id = a.role_id
WHERE a.person_id = sqlc.arg(person_id) OR a.group_id = ANY(sqlc.arg(group_ids)::uuid[])
ORDER BY r.name, p.type_name, p.action;

-- name: SetAccountPerson :exec
UPDATE account SET person_id = $2, updated_at = now() WHERE id = $1;
