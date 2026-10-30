-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The timeline (PL42, Hauptplan E121): one core table for the entries of every type with the
-- trait timeline. An entry is a text by its author, public or private, and never changes; a
-- correction is a new entry. Writing one does not raise the record's version; it leaves the
-- event comment. The entries go with the record when it is purged.

-- +goose Up
CREATE TABLE entry (
    id         uuid PRIMARY KEY,
    type_name  text NOT NULL,
    record_id  uuid NOT NULL,
    body       text NOT NULL CHECK (length(btrim(body)) > 0 AND length(body) <= 100000),
    private    boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid
);
CREATE INDEX entry_record_idx ON entry (type_name, record_id, created_at);

-- +goose Down
DROP TABLE entry;
