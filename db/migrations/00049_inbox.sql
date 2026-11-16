-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The inbox of a person (WF19, WF-Plan E19): a notification (core.notification) or the
-- notify of a rule leaves one entry per person and event, with subject and text in the
-- language of the person, next to or instead of the mail. Only the person reads and changes
-- her entries, through /api/v1/me/inbox; a job removes those older than the setting
-- notify.inbox_days (default 90 days), read or not. No type: an entry writes no history and
-- no event, which 70,000 persons would make expensive.

-- +goose Up
CREATE TABLE inbox_entry (
    id              uuid PRIMARY KEY,
    person_id       uuid NOT NULL,
    -- The notification it comes from; NULL for the notify of a rule.
    notification_id uuid,
    event_position  bigint NOT NULL,
    kind            text NOT NULL,
    type_name       text NOT NULL,
    record_id       uuid NOT NULL,
    -- The name of whoever caused the event, as the mail shows it.
    actor           text NOT NULL DEFAULT '',
    subject         text NOT NULL,
    body            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    read_at         timestamptz
);
-- A job that runs again (its end was not recorded) leaves no second entry.
CREATE UNIQUE INDEX inbox_entry_once ON inbox_entry (person_id, event_position, notification_id) NULLS NOT DISTINCT;
-- The page of a person, newest first, and the number of the unread.
CREATE INDEX inbox_entry_person ON inbox_entry (person_id, created_at DESC, id DESC);
CREATE INDEX inbox_entry_unread ON inbox_entry (person_id) WHERE read_at IS NULL;
-- The retention job.
CREATE INDEX inbox_entry_created ON inbox_entry (created_at);

-- +goose Down
DROP TABLE inbox_entry;
