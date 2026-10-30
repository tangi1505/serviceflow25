-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Events, part 3 (Hauptplan E87, revised 2026-10-26 after the measurement of LZ4): the
-- position comes from a sequence again, without a lock. Readers see only events whose
-- transaction is older than every transaction still in progress (the visibility horizon,
-- see db/queries/event.sql), so no event can appear behind one already read. Positions may
-- have gaps (a rolled-back transaction), which a reader with "after" never notices.

-- +goose Up
CREATE SEQUENCE event_position_seq OWNED BY event.position;
SELECT setval('event_position_seq', (SELECT last FROM event_counter) + 1, false);
ALTER TABLE event ALTER COLUMN position SET DEFAULT nextval('event_position_seq');
DROP TABLE event_counter;

-- +goose Down
CREATE TABLE event_counter (
    single boolean PRIMARY KEY DEFAULT true CHECK (single),
    last bigint NOT NULL
);
INSERT INTO event_counter (last) VALUES (COALESCE((SELECT max(position) FROM event), 0));
ALTER TABLE event ALTER COLUMN position DROP DEFAULT;
DROP SEQUENCE event_position_seq;
