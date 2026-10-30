-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Roles at the record (WF3; Inventar G37, G79; architektur.md B1): one core table for the
-- actors of every type with the trait actors. A type names its roles in its definition
-- (requester, observer, assigned, …) and which types may hold them (core.person, core.group,
-- a supplier type of a later package). The rows belong to the record's version: changing
-- them is an update of the record with audit rows and an event. The scope "involved" of a
-- permission reaches the records where the person or one of their groups holds a role.

-- +goose Up
CREATE TABLE actor (
    type_name   text NOT NULL,
    record_id   uuid NOT NULL,
    role        text NOT NULL,
    target_type text NOT NULL,
    target_id   uuid NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    created_by  uuid,
    PRIMARY KEY (type_name, record_id, role, target_type, target_id)
);
-- "Where am I or my groups involved": the scope involved and the filters @me, @my_groups.
CREATE INDEX actor_target_idx ON actor (target_id, type_name, role);

ALTER TABLE permission DROP CONSTRAINT permission_scope_check;
ALTER TABLE permission ADD CONSTRAINT permission_scope_check
    CHECK (scope IN ('own', 'group', 'involved', 'unit', 'subtree', 'all'));

-- +goose Down
DELETE FROM permission WHERE scope = 'involved';
ALTER TABLE permission DROP CONSTRAINT permission_scope_check;
ALTER TABLE permission ADD CONSTRAINT permission_scope_check
    CHECK (scope IN ('own', 'group', 'unit', 'subtree', 'all'));
DROP TABLE actor;
