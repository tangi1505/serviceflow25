-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Files (Hauptplan E22, E95; architektur.md §3.1; PL18): the metadata of every file in
-- PostgreSQL, the content in the file store behind an interface (file system now, S3 later).
-- A file is uploading while its content streams in, temporary until it is attached to a
-- record, stored afterwards. attachment links a file to a record of a type with the trait
-- documents; attaching and detaching leave audit rows (attach, detach) at the record.

-- +goose Up
CREATE TABLE file (
    id           uuid PRIMARY KEY,
    name         text NOT NULL,
    content_type text,
    size         bigint,
    sha256       text,
    state        text NOT NULL CHECK (state IN ('uploading', 'temporary', 'stored')),
    storage_key  text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   uuid
);
CREATE INDEX file_state_created_idx ON file (state, created_at);

CREATE TABLE attachment (
    id         uuid PRIMARY KEY,
    file_id    uuid NOT NULL REFERENCES file (id) ON DELETE CASCADE,
    type_name  text NOT NULL,
    record_id  uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    UNIQUE (file_id, type_name, record_id)
);
CREATE INDEX attachment_record_idx ON attachment (type_name, record_id);

ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach'));

-- +goose Down
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate'));
DROP TABLE attachment;
DROP TABLE file;
