-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: ClocksOfRecordForUpdate :many
SELECT * FROM clock WHERE record_id = sqlc.arg(record_id) ORDER BY started_at, id FOR UPDATE;

-- name: ClocksOfRecord :many
SELECT * FROM clock WHERE record_id = sqlc.arg(record_id) ORDER BY started_at, id;

-- name: ClockForUpdate :one
SELECT * FROM clock WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: InsertClock :exec
INSERT INTO clock (id, type_name, record_id, sla, state, target_seconds, calendar, started_at, used_seconds,
                   resumed_at, due_at, breached_at, stopped_at, generation, levels)
VALUES (sqlc.arg(id), sqlc.arg(type_name), sqlc.arg(record_id), sqlc.arg(sla), sqlc.arg(state), sqlc.arg(target_seconds),
        sqlc.narg(calendar), sqlc.arg(started_at), sqlc.arg(used_seconds), sqlc.narg(resumed_at), sqlc.narg(due_at),
        sqlc.narg(breached_at), sqlc.narg(stopped_at), sqlc.arg(generation), sqlc.arg(levels)::text[]);

-- name: UpdateClock :exec
UPDATE clock
SET state = sqlc.arg(state), target_seconds = sqlc.arg(target_seconds), calendar = sqlc.narg(calendar),
    started_at = sqlc.arg(started_at), used_seconds = sqlc.arg(used_seconds), resumed_at = sqlc.narg(resumed_at),
    due_at = sqlc.narg(due_at), breached_at = sqlc.narg(breached_at), stopped_at = sqlc.narg(stopped_at),
    generation = sqlc.arg(generation), levels = sqlc.arg(levels)::text[], updated_at = now()
WHERE id = sqlc.arg(id);

-- name: DeleteClocksOfRecord :exec
DELETE FROM clock WHERE record_id = sqlc.arg(record_id);
