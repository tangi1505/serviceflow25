-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Relations (Hauptplan KF2, E92; architektur.md §3.1; PL15): a core table with kind, source
-- and target, indexed in both directions. Graph queries walk it as a recursive CTE with a
-- depth limit and a path against cycles. Relating and unrelating leave audit rows at both
-- ends (actions relate, unrelate).

-- +goose Up
CREATE TABLE relation (
    id         uuid PRIMARY KEY,
    kind       text NOT NULL,
    from_type  text NOT NULL,
    from_id    uuid NOT NULL,
    to_type    text NOT NULL,
    to_id      uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    UNIQUE (kind, from_type, from_id, to_type, to_id),
    CHECK (from_type <> to_type OR from_id <> to_id)
);
CREATE INDEX relation_from ON relation (from_type, from_id, kind);
CREATE INDEX relation_to ON relation (to_type, to_id, kind);

ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate'));

-- +goose Down
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition'));
DROP TABLE relation;
