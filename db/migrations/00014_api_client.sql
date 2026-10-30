-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- API clients (Hauptplan E11, E90; PL13): a machine signs in with client id and secret
-- (OAuth 2 client credentials) and gets an opaque token held in Valkey. A client belongs to
-- an account without a local password, whose person carries the roles like any other.
-- ip_ranges holds CIDRs as text (empty = any address); NULL rate and ttl mean the settings
-- api.rate_limit and api.token_ttl of the root node.

-- +goose Up
CREATE TABLE api_client (
    client_id    text PRIMARY KEY CHECK (client_id ~ '^[a-z][a-z0-9_-]{0,62}$'),
    account_id   uuid NOT NULL UNIQUE REFERENCES account(id) ON DELETE CASCADE,
    name         text NOT NULL,
    secret_hash  text NOT NULL,
    ip_ranges    text[] NOT NULL DEFAULT '{}',
    rate_limit   integer CHECK (rate_limit IS NULL OR rate_limit BETWEEN 1 AND 1000000),
    token_ttl    integer CHECK (token_ttl IS NULL OR token_ttl BETWEEN 60 AND 2592000),
    enabled      boolean NOT NULL DEFAULT true,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    updated_by   uuid
);

-- +goose Down
DROP TABLE api_client;
