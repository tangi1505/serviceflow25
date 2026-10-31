-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- The directory of accounts in the control database (PL47b): which database an account lives
-- in. Kept with a database per tenant only.

-- name: ReserveAccountHome :exec
INSERT INTO account_home (account_id, username, tenant)
VALUES (sqlc.arg(account_id), sqlc.arg(username), sqlc.narg(tenant)::ltree);

-- name: GetAccountHome :one
SELECT account_id, username, coalesce(tenant::text, '')::text AS tenant, created_at
FROM account_home WHERE account_id = sqlc.arg(account_id);

-- name: GetAccountHomeByUsername :one
SELECT account_id, username, coalesce(tenant::text, '')::text AS tenant, created_at
FROM account_home WHERE username = sqlc.arg(username);

-- name: DeleteAccountHome :exec
DELETE FROM account_home WHERE account_id = sqlc.arg(account_id);

-- name: ListAccountHomes :many
SELECT account_id, username, coalesce(tenant::text, '')::text AS tenant, created_at
FROM account_home ORDER BY username;
