-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The dispatcher in batches (PL41, Hauptplan E112): every consumer of the outbox (live for
-- SSE, webhooks, rules) keeps its own position and state. A pass locks the consumer's row
-- (FOR UPDATE SKIP LOCKED, so that one instance at a time works a consumer), reads a batch,
-- writes what the consumer does and the new position in one transaction. A failure leaves
-- the position where it was and is kept here (last_error, failing_since, attempts) until the
-- next pass succeeds. The retention of events waits for the slowest durable consumer, so no
-- event goes before every consumer has taken it; consumers of tests are not durable. What
-- fails for good after all attempts of its job (a webhook delivery, a rule) is kept in
-- delivery_failure.

-- +goose Up
ALTER TABLE event_consumer
    ADD COLUMN durable       boolean NOT NULL DEFAULT true,
    ADD COLUMN last_error    text,
    ADD COLUMN failing_since timestamptz,
    ADD COLUMN attempts      integer NOT NULL DEFAULT 0;
-- The consumers of the dispatcher start where the one dispatcher stood.
INSERT INTO event_consumer (name, position)
SELECT c.name, coalesce((SELECT position FROM event_consumer WHERE name = 'dispatcher'),
                        (SELECT coalesce(max(position), 0) FROM event))
FROM (VALUES ('live'), ('webhooks'), ('rules')) AS c(name)
ON CONFLICT (name) DO NOTHING;
DELETE FROM event_consumer WHERE name = 'dispatcher';
-- Consumers of tests on a shared database must not hold the retention back.
UPDATE event_consumer SET durable = false WHERE name LIKE '%test%';

CREATE TABLE delivery_failure (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at             timestamptz NOT NULL DEFAULT now(),
    kind           text NOT NULL CHECK (kind IN ('webhook', 'rule')),
    target         text NOT NULL,
    event_position bigint,
    type_name      text,
    record_id      uuid,
    job_id         bigint,
    attempts       integer NOT NULL DEFAULT 0,
    error          text NOT NULL
);
CREATE INDEX delivery_failure_at_idx ON delivery_failure (at DESC, id DESC);

-- +goose Down
DROP TABLE delivery_failure;
INSERT INTO event_consumer (name, position)
SELECT 'dispatcher', coalesce(min(position), 0) FROM event_consumer WHERE name IN ('live', 'webhooks', 'rules')
ON CONFLICT (name) DO NOTHING;
DELETE FROM event_consumer WHERE name IN ('live', 'webhooks', 'rules');
ALTER TABLE event_consumer
    DROP COLUMN durable, DROP COLUMN last_error, DROP COLUMN failing_since, DROP COLUMN attempts;
