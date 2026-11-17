-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The day values of the dashboards (PL69, Hauptplan E195, Kern-Plan E73): per type, date field
-- (such as created_at or resolved_at), day in Europe/Berlin, node of the organisation and
-- group the number of records and the sums of number fields, so that long periods do not count
-- millions of rows. dim is the group ('' for all records of the day, a field, or actors.<role>),
-- key its value as JSON text ('null' for none). sums holds per number field [sum, count of
-- values]. internal/dayvalue keeps them: a consumer of the events marks the days a write
-- touched (day_value_dirty), a run every minute counts them again, and every night all days
-- are counted again. day_value_state says from when a type and date field are complete.

-- +goose Up
CREATE TABLE day_value (
    type_name  text NOT NULL,
    date_field text NOT NULL,
    day        date NOT NULL,
    org_path   ltree NOT NULL,
    dim        text NOT NULL,
    key        text NOT NULL,
    n          bigint NOT NULL,
    sums       jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY (type_name, date_field, dim, day, org_path, key)
);
CREATE INDEX day_value_org_path_idx ON day_value USING gist (org_path);

CREATE TABLE day_value_dirty (
    type_name  text NOT NULL,
    date_field text NOT NULL,
    day        date NOT NULL,
    PRIMARY KEY (type_name, date_field, day)
);

CREATE TABLE day_value_state (
    type_name  text NOT NULL,
    date_field text NOT NULL,
    -- the dims and number fields the day values hold, as the widgets asked when counted
    dims       text[] NOT NULL,
    measures   text[] NOT NULL,
    filled_at  timestamptz NOT NULL,
    PRIMARY KEY (type_name, date_field)
);

-- +goose Down
DROP TABLE day_value_state;
DROP TABLE day_value_dirty;
DROP TABLE day_value;
