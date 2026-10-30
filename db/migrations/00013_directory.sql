-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- LDAP directories (Hauptplan E10, E61, E89; PL12): several directories, each with its own
-- filters and attribute mapping; the bind password stays out of the database
-- (ITSM_LDAP_PASSWORD_<NAME>). directory_entry maps the stable id of an entry (entryUUID,
-- objectGUID) to the person or group it became, so that a rename stays the same record and
-- an entry that disappears is deactivated, not deleted.

-- +goose Up
CREATE TABLE ldap_directory (
    name          text PRIMARY KEY CHECK (name ~ '^[a-z][a-z0-9_-]{0,62}$'),
    url           text NOT NULL,
    bind_dn       text NOT NULL,
    base_dn       text NOT NULL,
    person_filter text NOT NULL DEFAULT '(objectClass=person)',
    group_filter  text NOT NULL DEFAULT '(objectClass=groupOfUniqueNames)',
    attributes    jsonb NOT NULL DEFAULT '{}'::jsonb,
    org_path      ltree NOT NULL DEFAULT 'root',
    start_tls     boolean NOT NULL DEFAULT false,
    skip_verify   boolean NOT NULL DEFAULT false,
    page_size     integer NOT NULL DEFAULT 500 CHECK (page_size BETWEEN 0 AND 10000),
    enabled       boolean NOT NULL DEFAULT true,
    last_sync_at  timestamptz,
    last_result   jsonb,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    updated_by    uuid
);

CREATE TABLE directory_entry (
    directory   text NOT NULL REFERENCES ldap_directory(name) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN ('person', 'group')),
    external_id text NOT NULL,
    record_id   uuid NOT NULL,
    dn          text NOT NULL,
    seen_at     timestamptz NOT NULL DEFAULT now(),
    active      boolean NOT NULL DEFAULT true,
    PRIMARY KEY (directory, kind, external_id)
);
CREATE INDEX directory_entry_record ON directory_entry (record_id);

-- +goose Down
DROP TABLE directory_entry;
DROP TABLE ldap_directory;
