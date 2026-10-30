-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Every change the core makes to a type table is recorded here (architektur.md §3.2).

-- +goose Up
CREATE TABLE schema_change (
    id         uuid PRIMARY KEY,
    type_name  text NOT NULL,
    change     text NOT NULL,
    statement  text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX schema_change_type ON schema_change (type_name, applied_at);

-- +goose Down
DROP TABLE schema_change;
