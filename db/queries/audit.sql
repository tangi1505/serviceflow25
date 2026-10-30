-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertAudit :exec
INSERT INTO audit (id, type_name, record_id, version, action, field, old_value, new_value, actor, rule)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: ListAudit :many
SELECT id, type_name, record_id, version, action, field, old_value, new_value, actor, at, rule
FROM audit
WHERE type_name = $1 AND record_id = $2
ORDER BY at, id;
