-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Events (Hauptplan E39, Workflow-Engine WF1): the outbox. A row is written in the same
-- transaction as the change it reports; the position is the order of the stream. WF1 writes
-- the transitions of the state model; PL24 adds the dispatcher, retention and SSE.

-- +goose Up
CREATE TABLE event (
    position   bigserial PRIMARY KEY,
    at         timestamptz NOT NULL DEFAULT now(),
    kind       text NOT NULL,
    type_name  text NOT NULL,
    record_id  uuid NOT NULL,
    actor      uuid,
    payload    jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX event_record ON event (type_name, record_id, position);

-- A transition of the state model is its own audit action.
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition'));

-- +goose Down
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge'));
DROP TABLE event;
