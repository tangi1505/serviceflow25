-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: ListDirectories :many
SELECT name, url, bind_dn, base_dn, person_filter, group_filter, attributes, org_path::text AS org_path,
       start_tls, skip_verify, page_size, enabled, last_sync_at, last_result, created_at, updated_at, updated_by
FROM ldap_directory
ORDER BY name;

-- name: GetDirectory :one
SELECT name, url, bind_dn, base_dn, person_filter, group_filter, attributes, org_path::text AS org_path,
       start_tls, skip_verify, page_size, enabled, last_sync_at, last_result, created_at, updated_at, updated_by
FROM ldap_directory
WHERE name = sqlc.arg(name);

-- name: InsertDirectory :exec
INSERT INTO ldap_directory (name, url, bind_dn, base_dn, person_filter, group_filter, attributes, org_path,
                            start_tls, skip_verify, page_size, enabled, updated_by)
VALUES (sqlc.arg(name), sqlc.arg(url), sqlc.arg(bind_dn), sqlc.arg(base_dn), sqlc.arg(person_filter),
        sqlc.arg(group_filter), sqlc.arg(attributes), sqlc.arg(org_path)::ltree, sqlc.arg(start_tls),
        sqlc.arg(skip_verify), sqlc.arg(page_size), sqlc.arg(enabled), sqlc.arg(updated_by));

-- name: UpdateDirectory :execrows
UPDATE ldap_directory
SET url = sqlc.arg(url), bind_dn = sqlc.arg(bind_dn), base_dn = sqlc.arg(base_dn),
    person_filter = sqlc.arg(person_filter), group_filter = sqlc.arg(group_filter),
    attributes = sqlc.arg(attributes), org_path = sqlc.arg(org_path)::ltree, start_tls = sqlc.arg(start_tls),
    skip_verify = sqlc.arg(skip_verify), page_size = sqlc.arg(page_size), enabled = sqlc.arg(enabled),
    updated_at = now(), updated_by = sqlc.arg(updated_by)
WHERE name = sqlc.arg(name);

-- name: DeleteDirectory :execrows
DELETE FROM ldap_directory WHERE name = sqlc.arg(name);

-- name: SetDirectoryResult :exec
UPDATE ldap_directory SET last_sync_at = now(), last_result = sqlc.arg(last_result) WHERE name = sqlc.arg(name);

-- name: ListDirectoryEntries :many
SELECT directory, kind, external_id, record_id, dn, seen_at, active
FROM directory_entry
WHERE directory = sqlc.arg(directory) AND kind = sqlc.arg(kind);

-- name: UpsertDirectoryEntry :exec
INSERT INTO directory_entry (directory, kind, external_id, record_id, dn, seen_at, active)
VALUES (sqlc.arg(directory), sqlc.arg(kind), sqlc.arg(external_id), sqlc.arg(record_id), sqlc.arg(dn), now(), true)
ON CONFLICT (directory, kind, external_id) DO UPDATE
    SET dn = EXCLUDED.dn, seen_at = now(), active = true;

-- name: DeactivateDirectoryEntry :exec
UPDATE directory_entry SET active = false
WHERE directory = sqlc.arg(directory) AND kind = sqlc.arg(kind) AND external_id = sqlc.arg(external_id);

-- name: AccountsOfPerson :many
SELECT * FROM account WHERE person_id = sqlc.arg(person_id);
