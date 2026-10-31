-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The control database (PL47b, Hauptplan E141; architektur.md §5.1). With a database per
-- tenant the installation keeps the directory of accounts, the login rules and the identities
-- of the identity providers, roles and permissions; each tenant keeps its accounts, its
-- assignments and its data. Every database has this schema; with one database it is both.
--
-- account_home names the tenant of every account, so that a login or a session finds the
-- database of the account before it knows anything else; the username is unique across the
-- installation (E138). It is kept with a database per tenant only: with one database every
-- account is at home in it.
--
-- Three foreign keys pointed across the line and go: assignments to a role or a login rule
-- (roles and login rules are central, assignments of a tenant's persons are in its database)
-- and identities to their account (identities are central, accounts of a tenant are not). What
-- their cascades did, the code does now: a role in use cannot be deleted (409), deleting a login
-- rule removes its assignments.
--
-- The security log pages by id; an administrator sees the entries of the tenant and of the
-- installation together, from two databases. Ids are therefore ordered by time across
-- databases: microseconds since 1970 times 1024 plus a random part (bigint holds that until
-- 2255), larger than any id before.

-- +goose Up
CREATE TABLE account_home (
    account_id uuid PRIMARY KEY,
    username   text NOT NULL UNIQUE,
    tenant     ltree,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX account_home_tenant_idx ON account_home (tenant);

ALTER TABLE identity ADD COLUMN tenant ltree;
UPDATE identity i SET tenant = a.tenant FROM account a WHERE a.id = i.account_id;

ALTER TABLE role_assignment DROP CONSTRAINT role_assignment_role_id_fkey;
ALTER TABLE role_assignment DROP CONSTRAINT role_assignment_rule_id_fkey;
CREATE INDEX role_assignment_role_idx ON role_assignment (role_id);
CREATE INDEX role_assignment_rule_idx ON role_assignment (rule_id) WHERE rule_id IS NOT NULL;
ALTER TABLE identity DROP CONSTRAINT identity_account_id_fkey;

ALTER TABLE security_event ALTER COLUMN id DROP IDENTITY;
ALTER TABLE security_event ALTER COLUMN id SET DEFAULT
    ((floor(extract(epoch FROM clock_timestamp()) * 1000000)::bigint << 10) | floor(random() * 1024)::bigint);

-- +goose Down
ALTER TABLE security_event ALTER COLUMN id DROP DEFAULT;
ALTER TABLE security_event ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;
ALTER TABLE identity ADD CONSTRAINT identity_account_id_fkey FOREIGN KEY (account_id) REFERENCES account (id) ON DELETE CASCADE;
DROP INDEX role_assignment_rule_idx;
DROP INDEX role_assignment_role_idx;
ALTER TABLE role_assignment ADD CONSTRAINT role_assignment_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES login_rule (id) ON DELETE CASCADE;
ALTER TABLE role_assignment ADD CONSTRAINT role_assignment_role_id_fkey FOREIGN KEY (role_id) REFERENCES role (id) ON DELETE CASCADE;
ALTER TABLE identity DROP COLUMN tenant;
DROP TABLE account_home;
