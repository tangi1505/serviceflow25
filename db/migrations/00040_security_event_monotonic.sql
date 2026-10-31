-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Monotonic ids of the security log (PL47e, Pascal's decision 2026-10-30, to E142). Since PL47b
-- an id is the time in microseconds times 1024 plus a random part, so that the entries of two
-- databases mix by id. When the clock of the database server steps back (in WSL several times
-- an hour, on virtual machines in operation too), a later entry got a smaller id and the list
-- showed it too far down. Now an id is at least the last one of this database plus one, else
-- the time id: within a database the order holds whatever the clock does, across databases it
-- stays roughly by time as before.
--
-- The last id lives in a table of one row; taking the next id locks that row until the insert
-- commits. The security log is written outside other transactions (seclog.Record), so the lock
-- is held for one insert.

-- +goose Up
CREATE TABLE security_event_last (
    one boolean PRIMARY KEY DEFAULT true CHECK (one),
    id  bigint NOT NULL
);
INSERT INTO security_event_last (id) SELECT coalesce(max(id), 0) FROM security_event;

-- +goose StatementBegin
CREATE FUNCTION security_event_next_id() RETURNS bigint LANGUAGE sql AS $$
    UPDATE security_event_last
    SET id = greatest(id + 1, (floor(extract(epoch FROM clock_timestamp()) * 1000000)::bigint << 10) | floor(random() * 1024)::bigint)
    RETURNING id
$$;
-- +goose StatementEnd

ALTER TABLE security_event ALTER COLUMN id SET DEFAULT security_event_next_id();

-- +goose Down
ALTER TABLE security_event ALTER COLUMN id SET DEFAULT
    ((floor(extract(epoch FROM clock_timestamp()) * 1000000)::bigint << 10) | floor(random() * 1024)::bigint);
DROP FUNCTION security_event_next_id();
DROP TABLE security_event_last;
