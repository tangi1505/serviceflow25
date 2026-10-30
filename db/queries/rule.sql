-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: ListRules :many
SELECT * FROM rule ORDER BY type_name, position, name;

-- name: GetRule :one
-- The rule of that name a tenant sees (PL45): its own, else one of the installation; without
-- a tenant only those of the installation.
SELECT * FROM rule WHERE name = sqlc.arg(name) AND (tenant IS NULL OR tenant = sqlc.narg(tenant)::ltree)
ORDER BY tenant NULLS LAST LIMIT 1;

-- name: RuleNameInTenants :one
-- Whether a tenant has a rule of that name: a rule of the installation may not take it.
SELECT EXISTS (SELECT 1 FROM rule WHERE name = sqlc.arg(name) AND tenant IS NOT NULL);

-- name: InsertRule :one
INSERT INTO rule (id, name, label, description, type_name, trigger, mode, condition, schedule, actions,
                  position, stop, enabled, sla, percent, minutes, created_by, updated_by, tenant)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(label), sqlc.arg(description), sqlc.arg(type_name), sqlc.arg(trigger),
        sqlc.arg(mode), sqlc.arg(condition), sqlc.arg(schedule), sqlc.arg(actions), sqlc.arg(position), sqlc.arg(stop),
        sqlc.arg(enabled), sqlc.narg(sla), sqlc.narg(percent), sqlc.narg(minutes), sqlc.narg(actor), sqlc.narg(actor),
        sqlc.narg(tenant)::ltree)
RETURNING *;

-- name: UpdateRule :one
UPDATE rule
SET label = sqlc.arg(label), description = sqlc.arg(description), type_name = sqlc.arg(type_name),
    trigger = sqlc.arg(trigger), mode = sqlc.arg(mode), condition = sqlc.arg(condition), schedule = sqlc.arg(schedule),
    actions = sqlc.arg(actions), position = sqlc.arg(position), stop = sqlc.arg(stop), enabled = sqlc.arg(enabled),
    sla = sqlc.narg(sla), percent = sqlc.narg(percent), minutes = sqlc.narg(minutes), version = version + 1, updated_at = now(), updated_by = sqlc.narg(actor)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteRule :execrows
DELETE FROM rule WHERE id = sqlc.arg(id);

-- name: SetRuleLastRun :exec
UPDATE rule SET last_run = sqlc.arg(last_run) WHERE id = sqlc.arg(id);
