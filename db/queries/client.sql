-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: ListClients :many
SELECT client_id, account_id, name, ip_ranges, rate_limit, token_ttl, enabled, last_used_at, created_at, updated_at, updated_by
FROM api_client
ORDER BY client_id;

-- name: GetClient :one
SELECT client_id, account_id, name, secret_hash, ip_ranges, rate_limit, token_ttl, enabled, last_used_at, created_at, updated_at, updated_by
FROM api_client
WHERE client_id = sqlc.arg(client_id);

-- name: InsertClient :exec
INSERT INTO api_client (client_id, account_id, name, secret_hash, ip_ranges, rate_limit, token_ttl, enabled, updated_by)
VALUES (sqlc.arg(client_id), sqlc.arg(account_id), sqlc.arg(name), sqlc.arg(secret_hash), sqlc.arg(ip_ranges),
        sqlc.narg(rate_limit), sqlc.narg(token_ttl), sqlc.arg(enabled), sqlc.arg(updated_by));

-- name: UpdateClient :execrows
UPDATE api_client
SET name = sqlc.arg(name), ip_ranges = sqlc.arg(ip_ranges), rate_limit = sqlc.narg(rate_limit),
    token_ttl = sqlc.narg(token_ttl), enabled = sqlc.arg(enabled), updated_at = now(), updated_by = sqlc.arg(updated_by)
WHERE client_id = sqlc.arg(client_id);

-- name: SetClientSecret :execrows
UPDATE api_client SET secret_hash = sqlc.arg(secret_hash), updated_at = now(), updated_by = sqlc.arg(updated_by)
WHERE client_id = sqlc.arg(client_id);

-- name: TouchClient :exec
UPDATE api_client SET last_used_at = now() WHERE client_id = sqlc.arg(client_id);

-- name: DeleteClient :execrows
DELETE FROM api_client WHERE client_id = sqlc.arg(client_id);
