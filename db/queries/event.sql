-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The visibility horizon (Hauptplan E87): a reader sees an event only when its transaction
-- is older than every transaction still in progress, so nothing can later appear behind a
-- position already read. age() handles the wraparound of transaction ids.

-- name: InsertEvent :one
INSERT INTO event (kind, type_name, record_id, actor, payload)
VALUES (sqlc.arg(kind), sqlc.arg(type_name), sqlc.arg(record_id), sqlc.narg(actor), sqlc.arg(payload))
RETURNING position, at;

-- name: ListEvents :many
SELECT position, at, kind, type_name, record_id, actor, payload
FROM event
WHERE position > $1
  AND age(xmin) > age(pg_snapshot_xmin(pg_current_snapshot())::text::xid)
ORDER BY position
LIMIT $2;

-- name: OldestEventPosition :one
SELECT coalesce(min(position), 0)::bigint AS oldest FROM event;

-- name: LatestEventPosition :one
SELECT coalesce(max(position), 0)::bigint AS latest
FROM event
WHERE age(xmin) > age(pg_snapshot_xmin(pg_current_snapshot())::text::xid);

-- name: DeleteEventsBefore :execrows
-- Only what every durable consumer has taken (PL41): a consumer that stands still holds the
-- retention back instead of losing events.
DELETE FROM event
WHERE at < sqlc.arg(before)
  AND position <= (SELECT coalesce(min(position), 9223372036854775807) FROM event_consumer WHERE durable);

-- name: GetConsumerPosition :one
SELECT position FROM event_consumer WHERE name = sqlc.arg(name);

-- name: SetConsumerPosition :exec
INSERT INTO event_consumer (name, position, updated_at)
VALUES (sqlc.arg(name), sqlc.arg(position), now())
ON CONFLICT (name) DO UPDATE SET position = EXCLUDED.position, updated_at = now();

-- name: LockConsumer :one
-- One pass of a consumer: the row stays locked until the pass commits (PL41).
SELECT position FROM event_consumer WHERE name = sqlc.arg(name) FOR UPDATE SKIP LOCKED;

-- name: EnsureConsumer :exec
INSERT INTO event_consumer (name, position, durable)
VALUES (sqlc.arg(name), sqlc.arg(position), sqlc.arg(durable))
ON CONFLICT (name) DO NOTHING;

-- name: ConsumerDone :exec
UPDATE event_consumer
SET position = sqlc.arg(position), updated_at = now(), last_error = NULL, failing_since = NULL, attempts = 0
WHERE name = sqlc.arg(name);

-- name: ConsumerFailed :one
UPDATE event_consumer
SET last_error = sqlc.arg(error), failing_since = coalesce(failing_since, now()), attempts = attempts + 1,
    updated_at = now()
WHERE name = sqlc.arg(name)
RETURNING failing_since, attempts;

-- name: ListConsumers :many
SELECT name, position, durable, last_error, failing_since, attempts, updated_at
FROM event_consumer ORDER BY name;

-- name: InsertDeliveryFailure :exec
-- The node comes from the event, so that a tenant sees its failures only (PL45).
INSERT INTO delivery_failure (kind, target, event_position, type_name, record_id, job_id, attempts, error, org_path)
VALUES (sqlc.arg(kind), sqlc.arg(target), sqlc.narg(event_position), sqlc.narg(type_name), sqlc.narg(record_id),
        sqlc.narg(job_id), sqlc.arg(attempts), sqlc.arg(error),
        (SELECT nullif(payload->>'org_path', '')::ltree FROM event WHERE position = sqlc.narg(event_position)));

-- name: ListDeliveryFailures :many
-- In a tenant its failures, and those without a node with installation (PL45).
SELECT id, at, kind, target, event_position, type_name, record_id, job_id, attempts, error
FROM delivery_failure
WHERE id < sqlc.arg(before)
  AND (sqlc.narg(tenant)::ltree IS NULL OR org_path <@ sqlc.narg(tenant)::ltree
       OR (org_path IS NULL AND sqlc.arg(installation)::boolean))
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);
