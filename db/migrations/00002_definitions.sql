-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Packages and definitions (architektur.md §4, Hauptplan E37, E66 to E68). A definition is one
-- JSONB document per kind, name and layer; the effective definition is merged from the layers.

-- +goose Up
CREATE TABLE package (
    name         text PRIMARY KEY,
    version      text NOT NULL,
    label        jsonb NOT NULL DEFAULT '{}'::jsonb,
    spec         jsonb NOT NULL,
    installed_at timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE definition (
    id         uuid PRIMARY KEY,
    package    text NOT NULL REFERENCES package(name) ON DELETE CASCADE,
    kind       text NOT NULL,
    name       text NOT NULL,
    layer      text NOT NULL CHECK (layer IN ('base', 'extension', 'customer')),
    spec       jsonb NOT NULL,
    version    bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (package, kind, name, layer)
);

CREATE INDEX definition_kind_name ON definition (kind, name);

-- +goose Down
DROP TABLE definition;
DROP TABLE package;
