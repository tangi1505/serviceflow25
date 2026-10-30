-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertCost :exec
INSERT INTO cost (id, type_name, record_id, kind, description, spent_on, person, minutes, rate, amount, created_by, updated_by)
VALUES (sqlc.arg(id), sqlc.arg(type_name), sqlc.arg(record_id), sqlc.arg(kind), sqlc.narg(description), sqlc.arg(spent_on),
        sqlc.narg(person), sqlc.narg(minutes), sqlc.narg(rate)::float8, sqlc.arg(amount)::float8,
        sqlc.narg(created_by), sqlc.narg(created_by));

-- name: GetCost :one
-- rate is shown for time entries only; the others have none (0 here).
SELECT id, kind, description, spent_on, person, minutes, coalesce(rate, 0)::float8 AS rate, amount::float8 AS amount,
       version, created_at, created_by, updated_at, updated_by
FROM cost
WHERE id = sqlc.arg(id) AND type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id);

-- name: UpdateCost :execrows
UPDATE cost
SET description = sqlc.narg(description), spent_on = sqlc.arg(spent_on), person = sqlc.narg(person),
    minutes = sqlc.narg(minutes), rate = sqlc.narg(rate)::float8, amount = sqlc.arg(amount)::float8,
    version = version + 1, updated_at = now(), updated_by = sqlc.narg(updated_by)
WHERE id = sqlc.arg(id) AND version = sqlc.arg(version);

-- name: DeleteCost :execrows
DELETE FROM cost WHERE id = sqlc.arg(id) AND type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id);

-- name: CostsOfRecord :many
SELECT id, kind, description, spent_on, person, minutes, coalesce(rate, 0)::float8 AS rate, amount::float8 AS amount,
       version, created_at, created_by, updated_at, updated_by
FROM cost
WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id)
ORDER BY spent_on, created_at, id;

-- name: DeleteCostsOfRecord :exec
DELETE FROM cost WHERE type_name = sqlc.arg(type_name) AND record_id = sqlc.arg(record_id);
