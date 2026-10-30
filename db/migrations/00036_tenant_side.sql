-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Side data per tenant (PL45, Hauptplan E136, E139; architektur.md §5.1). Rules belong to a
-- tenant or, without one, to the installation and act in every tenant; webhooks belong to a
-- tenant (the root while there are none); the security log and the failures of the dispatcher
-- carry the tenant of what they report. API clients follow their account (account.tenant),
-- directories and login rules their node. Select lists are copies per tenant: their keys are
-- unique per tenant, not per installation (V28).

-- +goose Up
ALTER TABLE rule ADD COLUMN tenant ltree REFERENCES org_node (path);
ALTER TABLE rule DROP CONSTRAINT rule_name_key;
CREATE UNIQUE INDEX rule_tenant_name_key ON rule (coalesce(tenant, 'root'::ltree), name);

ALTER TABLE webhook ADD COLUMN tenant ltree NOT NULL DEFAULT 'root';
CREATE INDEX webhook_tenant_idx ON webhook (tenant);

ALTER TABLE security_event ADD COLUMN tenant ltree;
CREATE INDEX security_event_tenant_idx ON security_event (tenant, id DESC);

ALTER TABLE delivery_failure ADD COLUMN org_path ltree;

DROP INDEX list_entry_key_key;
CREATE UNIQUE INDEX list_entry_key_key ON list_entry (subltree(org_path, 0, least(nlevel(org_path), 2)), list, key)
    WHERE deleted_at IS NULL;
DROP INDEX list_entry_source_key;
CREATE UNIQUE INDEX list_entry_source_key ON list_entry (subltree(org_path, 0, least(nlevel(org_path), 2)), list, source, external_key)
    WHERE source IS NOT NULL;

-- +goose Down
DROP INDEX list_entry_source_key;
CREATE UNIQUE INDEX list_entry_source_key ON list_entry (list, source, external_key) WHERE source IS NOT NULL;
DROP INDEX list_entry_key_key;
CREATE UNIQUE INDEX list_entry_key_key ON list_entry (list, key) WHERE deleted_at IS NULL;
ALTER TABLE delivery_failure DROP COLUMN org_path;
ALTER TABLE security_event DROP COLUMN tenant;
ALTER TABLE webhook DROP COLUMN tenant;
DROP INDEX rule_tenant_name_key;
ALTER TABLE rule ADD CONSTRAINT rule_name_key UNIQUE (name);
ALTER TABLE rule DROP COLUMN tenant;
