-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Several scopes per role, type and action (UI4, Hauptplan E124): a role may read its own
-- saved searches and those shared with it, or its own records and those it is involved in.
-- The lines of a role count with OR, as the lines of several roles always did.

-- +goose Up
ALTER TABLE permission DROP CONSTRAINT permission_role_id_type_name_action_key;
ALTER TABLE permission ADD CONSTRAINT permission_role_id_type_name_action_scope_key
    UNIQUE (role_id, type_name, action, scope);

-- +goose Down
-- Back to one line per role, type and action: the first line stays.
DELETE FROM permission p USING permission q
WHERE p.role_id = q.role_id AND p.type_name = q.type_name AND p.action = q.action AND p.id > q.id;
ALTER TABLE permission DROP CONSTRAINT permission_role_id_type_name_action_scope_key;
ALTER TABLE permission ADD CONSTRAINT permission_role_id_type_name_action_key UNIQUE (role_id, type_name, action);
