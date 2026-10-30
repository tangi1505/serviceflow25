-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Sign-in through an identity provider (Hauptplan E10, E61, E88; PL11): an identity links the
-- subject of an issuer to an account; an account without a local password has the empty hash.
-- login_rule assigns roles from the claims (G121); the assignments it made carry its id and
-- go when the rule no longer matches at the next sign-in.

-- +goose Up
CREATE TABLE identity (
    issuer        text NOT NULL,
    subject       text NOT NULL,
    account_id    uuid NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    email         text,
    claims        jsonb NOT NULL DEFAULT '{}'::jsonb,
    first_login_at timestamptz NOT NULL DEFAULT now(),
    last_login_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (issuer, subject),
    UNIQUE (issuer, account_id)
);

CREATE TABLE login_rule (
    id         uuid PRIMARY KEY,
    position   integer NOT NULL DEFAULT 100,
    name       text NOT NULL UNIQUE CHECK (name ~ '^[a-z][a-z0-9_-]{0,62}$'),
    condition  text NOT NULL,
    role_id    uuid NOT NULL REFERENCES role(id) ON DELETE CASCADE,
    org_path   ltree NOT NULL DEFAULT 'root',
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid
);

ALTER TABLE role_assignment ADD COLUMN rule_id uuid REFERENCES login_rule(id) ON DELETE CASCADE;
CREATE INDEX role_assignment_rule ON role_assignment (rule_id) WHERE rule_id IS NOT NULL;

-- +goose Down
ALTER TABLE role_assignment DROP COLUMN rule_id;
DROP TABLE login_rule;
DROP TABLE identity;
