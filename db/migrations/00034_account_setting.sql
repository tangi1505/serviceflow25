-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Personal settings (UI9, Hauptplan E131): one row per account and key, such as time_zone,
-- theme or the presets of a type (defaults.<type>). The language lives at the person; the
-- global defaults are settings of the organisation tree.

-- +goose Up
CREATE TABLE account_setting (
    account_id uuid NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z0-9_.]+)?$'),
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, key)
);

-- +goose Down
DROP TABLE account_setting;
