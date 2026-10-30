-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Roles and rights, part 1 (Plattform-Kern PL8; Hauptplan E34; architektur.md §5). A role is a
-- set of permissions (type, action, scope); an assignment gives a role to a person or a group
-- for a node of the organisation tree. Accounts link to a person so that a session knows whose
-- rights apply. The role "admin" is built in: every type, every action, scope all.

-- +goose Up
ALTER TABLE account ADD COLUMN person_id uuid;

CREATE TABLE role (
    id          uuid PRIMARY KEY,
    name        text NOT NULL UNIQUE CHECK (name ~ '^[a-z][a-z0-9_]{0,62}$'),
    label       jsonb NOT NULL DEFAULT '{}'::jsonb,
    description jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE permission (
    id        uuid PRIMARY KEY,
    role_id   uuid NOT NULL REFERENCES role(id) ON DELETE CASCADE,
    -- qualified type name, or * for every type
    type_name text NOT NULL,
    -- read, create, update, delete, a special right of a package, or * for every action
    action    text NOT NULL CHECK (action ~ '^[a-z][a-z0-9_]*$' OR action = '*'),
    scope     text NOT NULL CHECK (scope IN ('own', 'group', 'unit', 'subtree', 'all')),
    -- condition in CEL and field rights come with PL9
    condition text NOT NULL DEFAULT '',
    fields    jsonb,
    UNIQUE (role_id, type_name, action)
);

CREATE TABLE role_assignment (
    id         uuid PRIMARY KEY,
    role_id    uuid NOT NULL REFERENCES role(id) ON DELETE CASCADE,
    person_id  uuid,
    group_id   uuid,
    org_path   ltree NOT NULL DEFAULT 'root',
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    CHECK ((person_id IS NULL) <> (group_id IS NULL))
);

CREATE INDEX role_assignment_person ON role_assignment (person_id);
CREATE INDEX role_assignment_group ON role_assignment (group_id);

INSERT INTO role (id, name, label, description)
VALUES ('01a15a3c-0000-7000-8000-000000000001', 'admin',
        '{"de": "Administration", "en": "Administration"}',
        '{"de": "Jeder Typ, jede Aktion, überall.", "en": "Every type, every action, everywhere."}');
INSERT INTO permission (id, role_id, type_name, action, scope)
VALUES ('01a15a3c-0000-7000-8000-000000000002', '01a15a3c-0000-7000-8000-000000000001', '*', '*', 'all');

-- +goose Down
DROP TABLE role_assignment;
DROP TABLE permission;
DROP TABLE role;
ALTER TABLE account DROP COLUMN person_id;
