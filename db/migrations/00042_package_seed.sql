-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The start data of the packages (SD-Plan N9, E6, Hauptplan E160): which entry of a package's
-- data/ a tenant (or the root without tenants) got, and what became of it, a record id or the
-- name of a rule. An entry is created once; a row here means it was, even when the customer
-- has deleted the record since, so an update brings it not back. The rows live in the
-- database of the tenant, next to what they name.

-- +goose Up
CREATE TABLE package_seed (
    org_path   ltree       NOT NULL,
    package    text        NOT NULL,
    key        text        NOT NULL,
    target     text        NOT NULL,
    ref        text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_path, package, key)
);

-- +goose Down
DROP TABLE package_seed;
