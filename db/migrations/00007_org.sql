-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The organisation tree (Plattform-Kern PL6; Hauptplan E16, architektur.md §5): one tree per
-- installation as ltree paths, "root" at the top. Every type table carries org_path with a
-- GiST index (internal/schema). Settings per node inherit down the tree: the nearest ancestor
-- that sets a key wins.

-- +goose Up
CREATE TABLE org_node (
    path       ltree PRIMARY KEY,
    name       text NOT NULL CHECK (name ~ '^[a-z][a-z0-9_]{0,62}$'),
    label      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX org_node_path_gist ON org_node USING gist (path);

INSERT INTO org_node (path, name, label) VALUES ('root', 'root', '{"de": "Wurzel", "en": "Root"}');

CREATE TABLE org_setting (
    path       ltree NOT NULL REFERENCES org_node(path) ON DELETE CASCADE,
    key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$'),
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid,
    PRIMARY KEY (path, key)
);

-- +goose Down
DROP TABLE org_setting;
DROP TABLE org_node;
