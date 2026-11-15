-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The same role for the same holder at the same node once (PL57, Kern-Plan E67): found with
-- H14, a role assigned four times became four conditions of the rights filter. Doubles from
-- before go, the oldest stays.

-- +goose Up
DELETE FROM role_assignment a USING role_assignment b
WHERE a.role_id = b.role_id
  AND a.person_id IS NOT DISTINCT FROM b.person_id
  AND a.group_id IS NOT DISTINCT FROM b.group_id
  AND a.org_path = b.org_path
  AND (a.created_at, a.id) > (b.created_at, b.id);
CREATE UNIQUE INDEX role_assignment_unique ON role_assignment (role_id, person_id, group_id, org_path) NULLS NOT DISTINCT;

-- +goose Down
DROP INDEX role_assignment_unique;
