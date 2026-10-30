-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: AccountSettings :many
SELECT key, value FROM account_setting WHERE account_id = sqlc.arg(account_id) ORDER BY key;

-- name: UpsertAccountSetting :exec
INSERT INTO account_setting (account_id, key, value)
VALUES (sqlc.arg(account_id), sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (account_id, key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- name: DeleteAccountSetting :exec
DELETE FROM account_setting WHERE account_id = sqlc.arg(account_id) AND key = sqlc.arg(key);
