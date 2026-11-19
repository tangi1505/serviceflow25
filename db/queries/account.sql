-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: CreateAccount :one
INSERT INTO account (id, username, display_name, password_hash, password_expires_at, tenant)
VALUES (sqlc.arg(id), sqlc.arg(username), sqlc.arg(display_name), sqlc.arg(password_hash),
        sqlc.narg(password_expires_at), sqlc.narg(tenant)::ltree)
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

-- name: UpsertMirrorAccount :exec
-- The mirror of an account of the installation in a tenant's database (PL47b, E141): the same
-- id, name, display name, block and person, no password; signing in stays central.
INSERT INTO account (id, username, display_name, password_hash, disabled_at, person_id)
VALUES (sqlc.arg(id), sqlc.arg(username), sqlc.arg(display_name), '', sqlc.narg(disabled_at), sqlc.narg(person_id))
ON CONFLICT (id) DO UPDATE
    SET display_name = EXCLUDED.display_name, disabled_at = EXCLUDED.disabled_at,
        person_id = EXCLUDED.person_id, updated_at = now();

-- name: SetAccountLastLogin :exec
-- The moment of a successful local sign-in (PL77).
UPDATE account SET last_login_at = now() WHERE id = $1;

-- name: AccountIdentities :many
-- The identity providers an account signs in with and when it last did (PL77).
SELECT account_id, issuer, last_login_at FROM identity;

-- name: UpsertInvitation :exec
-- A new invitation replaces the one before (PL77).
INSERT INTO account_invitation (account_id, token_hash, email, expires_at, created_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (account_id) DO UPDATE
    SET token_hash = EXCLUDED.token_hash, email = EXCLUDED.email, expires_at = EXCLUDED.expires_at,
        created_at = now(), created_by = EXCLUDED.created_by;

-- name: GetInvitationByHash :one
SELECT i.account_id, i.expires_at, a.username
FROM account_invitation i JOIN account a ON a.id = i.account_id
WHERE i.token_hash = $1;

-- name: DeleteInvitation :exec
DELETE FROM account_invitation WHERE account_id = $1;

-- name: ListInvitations :many
SELECT account_id, email, expires_at FROM account_invitation;
