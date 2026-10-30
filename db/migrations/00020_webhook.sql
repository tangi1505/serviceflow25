-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Webhooks (Hauptplan E39, KF13, E100; PL25; GLPI Webhook): a subscription sends the events
-- its owner may read to a URL, signed after Standard Webhooks. position is the last event
-- handed to the delivery queue, so that a dispatcher that runs an event twice does not
-- deliver it twice. The secret signs; it is shown once when created or rotated.

-- +goose Up
CREATE TABLE webhook (
    id         uuid PRIMARY KEY,
    name       text NOT NULL,
    url        text NOT NULL,
    secret     text NOT NULL,
    owner      uuid NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    kinds      text[] NOT NULL DEFAULT '{}',
    types      text[] NOT NULL DEFAULT '{}',
    enabled    boolean NOT NULL DEFAULT true,
    position   bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid
);
CREATE INDEX webhook_enabled_idx ON webhook (enabled) WHERE enabled;

-- +goose Down
DROP TABLE webhook;
