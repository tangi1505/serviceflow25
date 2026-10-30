-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: ListJobSchedules :many
SELECT name, schedule, enabled, updated_at, updated_by
FROM job_schedule
ORDER BY name;

-- name: GetJobSchedule :one
SELECT name, schedule, enabled, updated_at, updated_by
FROM job_schedule
WHERE name = sqlc.arg(name);

-- name: UpsertJobSchedule :one
INSERT INTO job_schedule (name, schedule, enabled, updated_at, updated_by)
VALUES (sqlc.arg(name), sqlc.arg(schedule), sqlc.arg(enabled), now(), sqlc.arg(updated_by))
ON CONFLICT (name) DO UPDATE
    SET schedule = EXCLUDED.schedule, enabled = EXCLUDED.enabled,
        updated_at = now(), updated_by = EXCLUDED.updated_by
RETURNING name, schedule, enabled, updated_at, updated_by;

-- name: DeleteJobSchedule :exec
DELETE FROM job_schedule WHERE name = sqlc.arg(name);
