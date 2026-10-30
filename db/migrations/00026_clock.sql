-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Deadlines and SLA clocks (WF9; Hauptplan KF5; architektur.md §8). An SLA is a record of the
-- core type core.sla; a clock is the running deadline of one SLA at one record, in this core
-- table: no versions and no rights of its own, whoever reads the record reads its clocks.
-- A clock counts working time of its calendar (WF8): used is what it counted up to resumed_at,
-- the start of the current run; due_at is when the target is reached if it keeps running.
-- A River job waits for due_at (generation says which job is the current one); there is no
-- run over all clocks. A breach leaves an audit row and an event "breach" at the record.

-- +goose Up
CREATE TABLE clock (
    id             uuid PRIMARY KEY,
    type_name      text NOT NULL,
    record_id      uuid NOT NULL,
    sla            uuid NOT NULL,
    state          text NOT NULL CHECK (state IN ('running', 'paused', 'stopped', 'cancelled')),
    target_seconds bigint NOT NULL CHECK (target_seconds > 0),
    calendar       uuid,
    started_at     timestamptz NOT NULL,
    used_seconds   bigint NOT NULL DEFAULT 0,
    resumed_at     timestamptz,
    due_at         timestamptz,
    breached_at    timestamptz,
    stopped_at     timestamptz,
    generation     bigint NOT NULL DEFAULT 1,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (record_id, sla),
    CHECK ((state = 'running') = (resumed_at IS NOT NULL AND due_at IS NOT NULL))
);
CREATE INDEX clock_due_idx ON clock (due_at) WHERE state = 'running' AND breached_at IS NULL;

ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach',
                      'cost_add', 'cost_change', 'cost_remove', 'notify', 'breach'));

-- +goose Down
DELETE FROM audit WHERE action = 'breach';
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach',
                      'cost_add', 'cost_change', 'cost_remove', 'notify'));
DROP TABLE clock;
