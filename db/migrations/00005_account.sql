-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Local accounts (Plattform-Kern PL5; Hauptplan E10, E73, E75). Persons from LDAP and OIDC come
-- with PL7 and PL9 and will reference an account only when they can sign in locally.

-- +goose Up
CREATE TABLE account (
    id                  uuid PRIMARY KEY,
    username            text NOT NULL UNIQUE CHECK (username ~ '^[a-z][a-z0-9_]{0,62}$'),
    display_name        text NOT NULL,
    password_hash       text NOT NULL,
    password_changed_at timestamptz NOT NULL DEFAULT now(),
    password_expires_at timestamptz,
    disabled_at         timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE account;
