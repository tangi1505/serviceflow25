-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertWebhook :one
INSERT INTO webhook (id, name, url, secret, owner, kinds, types, enabled, position, created_by, updated_by, tenant)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(url), sqlc.arg(secret), sqlc.arg(owner), sqlc.arg(kinds), sqlc.arg(types),
        sqlc.arg(enabled), sqlc.arg(position), sqlc.narg(created_by), sqlc.narg(created_by), sqlc.arg(tenant)::ltree)
RETURNING *;

-- name: GetWebhook :one
SELECT * FROM webhook WHERE id = sqlc.arg(id);

-- name: ListWebhooks :many
SELECT * FROM webhook ORDER BY name, id;

-- name: UpdateWebhook :one
UPDATE webhook
SET name = sqlc.arg(name), url = sqlc.arg(url), kinds = sqlc.arg(kinds), types = sqlc.arg(types),
    enabled = sqlc.arg(enabled), updated_at = now(), updated_by = sqlc.narg(updated_by)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetWebhookSecret :one
UPDATE webhook SET secret = sqlc.arg(secret), updated_at = now(), updated_by = sqlc.narg(updated_by)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteWebhook :execrows
DELETE FROM webhook WHERE id = sqlc.arg(id);

-- name: DueWebhooks :many
-- The enabled subscriptions that have not yet taken this event and whose filters match it.
SELECT * FROM webhook
WHERE enabled AND position < sqlc.arg(position)
  AND (cardinality(kinds) = 0 OR sqlc.arg(kind)::text = ANY(kinds))
  AND (cardinality(types) = 0 OR sqlc.arg(type_name)::text = ANY(types))
ORDER BY id;

-- name: AdvanceWebhook :execrows
UPDATE webhook SET position = sqlc.arg(position) WHERE id = sqlc.arg(id) AND position < sqlc.arg(position);

-- name: GetEvent :one
SELECT position, at, kind, type_name, record_id, actor, payload FROM event WHERE position = sqlc.arg(position);

-- name: EnabledWebhooks :many
-- The subscriptions a batch of the dispatcher matches in memory (PL41).
SELECT * FROM webhook WHERE enabled ORDER BY id;

-- name: WebhookPositions :many
-- Where the subscriptions of the installation stand in this tenant's database (PL47d).
SELECT webhook_id, position FROM webhook_position;

-- name: SetWebhookPosition :exec
INSERT INTO webhook_position (webhook_id, position) VALUES (sqlc.arg(webhook_id), sqlc.arg(position))
ON CONFLICT (webhook_id) DO UPDATE SET position = excluded.position WHERE webhook_position.position < excluded.position;

-- name: DeleteWebhookPosition :exec
DELETE FROM webhook_position WHERE webhook_id = sqlc.arg(webhook_id);
