-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Approvals (WF6, WF-Plan E17; KF4; Inventar G43, G44): one core set of tables for the records
-- of every type with the trait approvals. An approval has stages one after the other; a stage
-- asks persons and groups at once and needs a least number of yes (0: all of them). A group is
-- asked once: the first of its members decides for it. A person may name substitutes for a
-- time; they decide for her, and for her groups, while it lasts. The approvals go with the
-- record when it is purged; the history keeps every decision (audit action approval).

-- +goose Up
CREATE TABLE approval (
    id           uuid        PRIMARY KEY,
    type_name    text        NOT NULL,
    record_id    uuid        NOT NULL,
    state        text        NOT NULL CHECK (state IN ('open', 'approved', 'rejected', 'withdrawn')),
    stage        integer     NOT NULL DEFAULT 1,
    message      text        NOT NULL DEFAULT '',
    requested_by uuid,
    requester    uuid,
    requested_at timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz
);
CREATE INDEX approval_record ON approval (type_name, record_id);
-- At most one open approval per record.
CREATE UNIQUE INDEX approval_one_open ON approval (type_name, record_id) WHERE state = 'open';

CREATE TABLE approval_stage (
    approval_id uuid    NOT NULL REFERENCES approval (id) ON DELETE CASCADE,
    position    integer NOT NULL CHECK (position > 0),
    name        text    NOT NULL DEFAULT '',
    need        integer NOT NULL DEFAULT 0 CHECK (need >= 0),
    state       text    NOT NULL CHECK (state IN ('waiting', 'open', 'approved', 'rejected', 'dropped')),
    PRIMARY KEY (approval_id, position)
);

CREATE TABLE approval_step (
    id            uuid        PRIMARY KEY,
    approval_id   uuid        NOT NULL REFERENCES approval (id) ON DELETE CASCADE,
    stage         integer     NOT NULL,
    position      integer     NOT NULL,
    type_name     text        NOT NULL,
    record_id     uuid        NOT NULL,
    approver_type text        NOT NULL CHECK (approver_type IN ('core.person', 'core.group')),
    approver_id   uuid        NOT NULL,
    state         text        NOT NULL CHECK (state IN ('waiting', 'open', 'approved', 'rejected', 'dropped')),
    comment       text        NOT NULL DEFAULT '',
    decided_by    uuid,
    on_behalf     uuid,
    asked_at      timestamptz,
    decided_at    timestamptz,
    UNIQUE (approval_id, stage, approver_type, approver_id),
    FOREIGN KEY (approval_id, stage) REFERENCES approval_stage (approval_id, position) ON DELETE CASCADE
);
-- Who is asked: the list "Zu genehmigen" and the right to read the record.
CREATE INDEX approval_step_approver ON approval_step (approver_id, state);
CREATE INDEX approval_step_record ON approval_step (type_name, record_id);

CREATE TABLE substitute (
    id            uuid        PRIMARY KEY,
    person_id     uuid        NOT NULL,
    substitute_id uuid        NOT NULL,
    valid_from    timestamptz NOT NULL,
    valid_until   timestamptz,
    created_by    uuid,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (person_id <> substitute_id),
    CHECK (valid_until IS NULL OR valid_until > valid_from)
);
CREATE INDEX substitute_person ON substitute (person_id);
CREATE INDEX substitute_substitute ON substitute (substitute_id);

ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach',
                      'cost_add', 'cost_change', 'cost_remove', 'notify', 'breach', 'escalation', 'approval'));

-- +goose Down
DELETE FROM audit WHERE action = 'approval';
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach',
                      'cost_add', 'cost_change', 'cost_remove', 'notify', 'breach', 'escalation'));
DROP TABLE substitute;
DROP TABLE approval_step;
DROP TABLE approval_stage;
DROP TABLE approval;
