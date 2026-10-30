-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Events, part 2 (Hauptplan E39, E46, E87; PL24): the position comes from a counter row that
-- the writing transaction locks as its last step, so positions are gapless and in commit
-- order (a subscriber that reads after position n never misses n+1). event_consumer holds
-- the position of the dispatcher in the worker.

-- +goose Up
CREATE TABLE event_counter (
    single boolean PRIMARY KEY DEFAULT true CHECK (single),
    last bigint NOT NULL
);
INSERT INTO event_counter (last) VALUES (COALESCE((SELECT max(position) FROM event), 0));
ALTER TABLE event ALTER COLUMN position DROP DEFAULT;
DROP SEQUENCE IF EXISTS event_position_seq;

CREATE TABLE event_consumer (
    name       text PRIMARY KEY,
    position   bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE event_consumer;
CREATE SEQUENCE event_position_seq OWNED BY event.position;
SELECT setval('event_position_seq', (SELECT last FROM event_counter) + 1, false);
ALTER TABLE event ALTER COLUMN position SET DEFAULT nextval('event_position_seq');
DROP TABLE event_counter;
