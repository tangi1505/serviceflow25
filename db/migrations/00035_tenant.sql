-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Tenants (PL44, Hauptplan E136, architektur.md §5.1): a tenant is a node directly below the
-- root marked tenant. Without such a node the root is the only tenant and everything works as
-- before. An account with a tenant belongs to it alone; an account without one belongs to the
-- installation and works in every tenant where it holds a role, one at a time.

-- +goose Up
ALTER TABLE org_node ADD COLUMN tenant boolean NOT NULL DEFAULT false;
ALTER TABLE org_node ADD CONSTRAINT org_node_tenant_level CHECK (NOT tenant OR nlevel(path) = 2);
ALTER TABLE account ADD COLUMN tenant ltree REFERENCES org_node (path);
CREATE INDEX account_tenant_idx ON account (tenant);

-- +goose Down
ALTER TABLE account DROP COLUMN tenant;
ALTER TABLE org_node DROP CONSTRAINT org_node_tenant_level;
ALTER TABLE org_node DROP COLUMN tenant;
