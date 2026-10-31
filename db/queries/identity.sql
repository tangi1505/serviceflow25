-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: GetIdentity :one
SELECT issuer, subject, account_id, coalesce(tenant::text, '')::text AS tenant, email, claims, first_login_at, last_login_at
FROM identity
WHERE issuer = sqlc.arg(issuer) AND subject = sqlc.arg(subject);

-- name: UpsertIdentity :exec
-- The identity names the tenant of its account (PL47b): it is central, the account may not be.
INSERT INTO identity (issuer, subject, account_id, tenant, email, claims)
VALUES (sqlc.arg(issuer), sqlc.arg(subject), sqlc.arg(account_id), sqlc.narg(tenant)::ltree, sqlc.narg(email), sqlc.arg(claims))
ON CONFLICT (issuer, subject) DO UPDATE
    SET email = EXCLUDED.email, claims = EXCLUDED.claims, last_login_at = now();

-- name: InsertExternalAccount :one
INSERT INTO account (id, username, display_name, password_hash)
VALUES (sqlc.arg(id), sqlc.arg(username), sqlc.arg(display_name), '')
RETURNING *;

-- name: UsernameTaken :one
SELECT EXISTS (SELECT 1 FROM account WHERE username = sqlc.arg(username)) AS taken;

-- name: ListLoginRules :many
SELECT l.id, l.position, l.name, l.condition, l.role_id, r.name AS role_name, l.org_path::text AS org_path,
       l.enabled, l.created_at, l.updated_at, l.updated_by
FROM login_rule l JOIN role r ON r.id = l.role_id
ORDER BY l.position, l.name;

-- name: GetLoginRule :one
SELECT l.id, l.position, l.name, l.condition, l.role_id, r.name AS role_name, l.org_path::text AS org_path,
       l.enabled, l.created_at, l.updated_at, l.updated_by
FROM login_rule l JOIN role r ON r.id = l.role_id
WHERE l.id = sqlc.arg(id);

-- name: InsertLoginRule :exec
INSERT INTO login_rule (id, position, name, condition, role_id, org_path, enabled, updated_by)
VALUES (sqlc.arg(id), sqlc.arg(position), sqlc.arg(name), sqlc.arg(condition), sqlc.arg(role_id),
        sqlc.arg(org_path)::ltree, sqlc.arg(enabled), sqlc.arg(updated_by));

-- name: UpdateLoginRule :execrows
UPDATE login_rule
SET position = sqlc.arg(position), name = sqlc.arg(name), condition = sqlc.arg(condition),
    role_id = sqlc.arg(role_id), org_path = sqlc.arg(org_path)::ltree, enabled = sqlc.arg(enabled),
    updated_at = now(), updated_by = sqlc.arg(updated_by)
WHERE id = sqlc.arg(id);

-- name: DeleteLoginRule :execrows
DELETE FROM login_rule WHERE id = sqlc.arg(id);

-- name: ListRuleAssignmentsOfPerson :many
SELECT id, rule_id, role_id, org_path::text AS org_path
FROM role_assignment
WHERE person_id = sqlc.arg(person_id) AND rule_id IS NOT NULL;

-- name: InsertRuleAssignment :exec
INSERT INTO role_assignment (id, role_id, person_id, org_path, rule_id)
VALUES (sqlc.arg(id), sqlc.arg(role_id), sqlc.arg(person_id), sqlc.arg(org_path)::ltree, sqlc.arg(rule_id));
