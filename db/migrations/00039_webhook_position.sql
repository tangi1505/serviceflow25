-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Webhooks of the installation across databases (PL47d, Hauptplan E144). A subscription made at
-- the root lies in the control database and receives the events of every tenant its owner may
-- read, in both operating modes. With a database per tenant each tenant's database has its own
-- stream, so the subscription stands at a position in each: webhook_position keeps it, in the
-- tenant's database, beside the stream it counts in. The subscription's own position column
-- stays the one in the control database. There is no foreign key: the subscription lies in the
-- other database; deleting it removes its positions in every database.

-- +goose Up
CREATE TABLE webhook_position (
    webhook_id uuid PRIMARY KEY,
    position   bigint NOT NULL
);

-- +goose Down
DROP TABLE webhook_position;
