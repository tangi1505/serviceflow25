-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Rules "if, then" (WF4; Hauptplan KF7; architektur.md B4, §6.2): one core table for the rules
-- of every type. A rule fires on create, update (transitions included) or a schedule (time);
-- its condition is CEL over the record and the changed fields. A rule "before" changes the
-- record before it is written, in the same transaction (set, assign); a rule "after" runs as a
-- job once the write is done (set, assign, create, notify). Rules act with the rights of the
-- system; the history names the rule in the new column audit.rule. The action notify leaves an
-- audit row and an event of its own (WF15 delivers it by mail).

-- +goose Up
CREATE TABLE rule (
    id          uuid PRIMARY KEY,
    name        text NOT NULL UNIQUE CHECK (name ~ '^[a-z][a-z0-9_]{0,62}$'),
    label       jsonb NOT NULL DEFAULT '{}',
    description jsonb NOT NULL DEFAULT '{}',
    type_name   text NOT NULL,
    trigger     text NOT NULL CHECK (trigger IN ('create', 'update', 'time')),
    mode        text NOT NULL CHECK (mode IN ('before', 'after')),
    condition   text NOT NULL DEFAULT '',
    schedule    text NOT NULL DEFAULT '',
    actions     jsonb NOT NULL,
    position    integer NOT NULL DEFAULT 0,
    stop        boolean NOT NULL DEFAULT false,
    enabled     boolean NOT NULL DEFAULT true,
    last_run    timestamptz,
    version     bigint NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    created_by  uuid,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  uuid,
    -- A rule before the write cannot wait for a schedule.
    CHECK (trigger <> 'time' OR mode = 'after'),
    CHECK ((trigger = 'time') = (schedule <> ''))
);
CREATE INDEX rule_type_idx ON rule (type_name, trigger, mode, position, name);

ALTER TABLE audit ADD COLUMN rule text;
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach',
                      'cost_add', 'cost_change', 'cost_remove', 'notify'));

-- +goose Down
DELETE FROM audit WHERE action = 'notify';
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach',
                      'cost_add', 'cost_change', 'cost_remove'));
ALTER TABLE audit DROP COLUMN rule;
DROP TABLE rule;
