-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The audit trail: one row per changed field (architektur.md §2 "Audit", §3.3). Partitioned by
-- month from the start (Hauptplan E47); PL39 creates the monthly partitions, until then the
-- default partition takes every row.

-- +goose Up
CREATE TABLE audit (
    id         uuid NOT NULL,
    type_name  text NOT NULL,
    record_id  uuid NOT NULL,
    version    bigint NOT NULL,
    action     text NOT NULL CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge')),
    field      text,
    old_value  jsonb,
    new_value  jsonb,
    actor      uuid,
    at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, at)
) PARTITION BY RANGE (at);

CREATE TABLE audit_default PARTITION OF audit DEFAULT;

CREATE INDEX audit_record ON audit (type_name, record_id, at);

-- +goose Down
DROP TABLE audit;
