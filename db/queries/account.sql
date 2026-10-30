-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: CreateAccount :one
INSERT INTO account (id, username, display_name, password_hash, password_expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM account WHERE id = $1;

-- name: GetAccountByUsername :one
SELECT * FROM account WHERE username = $1;

-- name: ListAccounts :many
SELECT * FROM account ORDER BY username;

-- name: SetAccountTenant :exec
-- Binds an account to a tenant (PL44); NULL makes it an account of the installation.
UPDATE account SET tenant = sqlc.narg(tenant)::ltree, updated_at = now() WHERE id = sqlc.arg(id);

-- name: SetAccountPassword :exec
UPDATE account
SET password_hash = $2, password_changed_at = now(), password_expires_at = $3, updated_at = now()
WHERE id = $1;

-- name: SetAccountDisabled :exec
UPDATE account SET disabled_at = $2, updated_at = now() WHERE id = $1;
