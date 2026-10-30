-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertSecurityEvent :exec
INSERT INTO security_event (category, kind, outcome, account, username, ip, user_agent, reason, target, details, tenant)
VALUES (sqlc.arg(category), sqlc.arg(kind), sqlc.arg(outcome), sqlc.narg(account), sqlc.narg(username),
        sqlc.narg(ip)::inet, sqlc.narg(user_agent), sqlc.narg(reason), sqlc.narg(target), sqlc.arg(details),
        sqlc.narg(tenant)::ltree);

-- name: ListSecurityEvents :many
-- Newest first, by key: before is the id of the last entry of the previous page.
-- In a tenant its entries, and those of the installation (no tenant) with installation (PL45).
SELECT id, at, category, kind, outcome, account, username, coalesce(host(ip), '')::text AS ip, user_agent, reason, target, details,
       coalesce(tenant::text, '')::text AS tenant
FROM security_event
WHERE category = sqlc.arg(category)
  AND (sqlc.narg(tenant)::ltree IS NULL OR tenant = sqlc.narg(tenant)::ltree
       OR (tenant IS NULL AND sqlc.arg(installation)::boolean))
  AND (sqlc.narg(before)::bigint IS NULL OR id < sqlc.narg(before))
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
  AND (sqlc.narg(outcome)::text IS NULL OR outcome = sqlc.narg(outcome))
  AND (sqlc.narg(account)::uuid IS NULL OR account = sqlc.narg(account))
  AND (sqlc.narg(username)::text IS NULL OR username = sqlc.narg(username))
  AND (sqlc.narg(ip)::inet IS NULL OR ip = sqlc.narg(ip))
  AND (sqlc.narg(since)::timestamptz IS NULL OR at >= sqlc.narg(since))
  AND (sqlc.narg(until)::timestamptz IS NULL OR at < sqlc.narg(until))
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);

-- name: DeleteSecurityEventsBefore :execrows
DELETE FROM security_event WHERE at < sqlc.arg(before);
