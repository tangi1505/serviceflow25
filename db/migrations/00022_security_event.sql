-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Security event log (Hauptplan E102; PL27; GLPI Event, Inventar G128): sign-ins on every
-- way with success and failure, and changes of accounts, rights, login rules, API clients,
-- directories and settings. Append-only; a nightly job removes what is older than the setting
-- audit.security_retention (365 days by default).

-- +goose Up
CREATE TABLE security_event (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at         timestamptz NOT NULL DEFAULT now(),
    category   text NOT NULL CHECK (category IN ('auth', 'change')),
    kind       text NOT NULL,
    outcome    text NOT NULL CHECK (outcome IN ('success', 'failure')),
    account    uuid,
    username   text,
    ip         inet,
    user_agent text,
    reason     text,
    target     text,
    details    jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX security_event_category_idx ON security_event (category, id DESC);
CREATE INDEX security_event_account_idx ON security_event (account, id DESC);
CREATE INDEX security_event_username_idx ON security_event (username, id DESC);
CREATE INDEX security_event_at_idx ON security_event (at);

-- +goose Down
DROP TABLE security_event;
