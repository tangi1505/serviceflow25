-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Costs and times (Hauptplan KF18, E97; architektur.md B1; PL20): one core table for the
-- cost entries of every type with the trait costs. An entry is time (minutes at an hourly
-- rate), fixed or material. The sums live as the read-only fields time_spent and cost_total
-- on the record's row and are kept in the same transaction as the entry; booking does not
-- raise the record's version. Booking, changing and removing leave audit rows at the record.

-- +goose Up
CREATE TABLE cost (
    id          uuid PRIMARY KEY,
    type_name   text NOT NULL,
    record_id   uuid NOT NULL,
    kind        text NOT NULL CHECK (kind IN ('time', 'fixed', 'material')),
    description text,
    spent_on    date NOT NULL,
    person      uuid,
    minutes     integer CHECK (minutes > 0),
    rate        numeric(12, 2) CHECK (rate >= 0),
    amount      numeric(14, 2) NOT NULL CHECK (amount >= 0),
    version     bigint NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    created_by  uuid,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  uuid,
    -- Minutes and rate belong to time entries only.
    CHECK ((kind = 'time') = (minutes IS NOT NULL)),
    CHECK (kind = 'time' OR rate IS NULL)
);
CREATE INDEX cost_record_idx ON cost (type_name, record_id, spent_on);

ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach',
                      'cost_add', 'cost_change', 'cost_remove'));

-- +goose Down
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach'));
DROP TABLE cost;
