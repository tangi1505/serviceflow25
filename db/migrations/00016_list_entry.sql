-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Select lists (architektur.md B3; Hauptplan E93; PL16): one core table for the entries of
-- every list instead of a table per list. The column list names the list type
-- (package.type); the other columns are the fields the trait "list" gives every list. A
-- tree keeps path (ltree of ids) and full_name ("Country > City") up to date on every write.
-- The system columns are those of every type table (architektur.md §3.1).

-- +goose Up
CREATE TABLE list_entry (
    id           uuid PRIMARY KEY,
    org_path     ltree NOT NULL DEFAULT 'root',
    version      bigint NOT NULL DEFAULT 1,
    state        text,
    source       text,
    external_key text,
    row_hash     bytea,
    search       tsvector,
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   uuid,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    updated_by   uuid,
    deleted_at   timestamptz,
    list         text NOT NULL,
    name         text NOT NULL,
    key          text,
    description  text,
    active       boolean,
    position     numeric,
    parent       uuid,
    path         ltree,
    full_name    text,
    lat          numeric,
    lon          numeric,
    alt          numeric
);
CREATE INDEX list_entry_list_name_idx ON list_entry (list, name);
CREATE UNIQUE INDEX list_entry_key_key ON list_entry (list, key) WHERE deleted_at IS NULL;
CREATE INDEX list_entry_parent_idx ON list_entry (parent);
CREATE INDEX list_entry_path_idx ON list_entry USING gist (path);
CREATE INDEX list_entry_org_path_idx ON list_entry USING gist (org_path);
CREATE UNIQUE INDEX list_entry_source_key ON list_entry (list, source, external_key) WHERE source IS NOT NULL;
CREATE INDEX list_entry_updated_at_idx ON list_entry (updated_at);

-- +goose Down
DROP TABLE list_entry;
