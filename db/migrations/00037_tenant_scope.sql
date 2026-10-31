-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The tenant of a row (PL46, Hauptplan E136; architektur.md §5.1). Unique fields and external
-- keys are unique per tenant: the same serial number in A and B, not twice in A. A column
-- tenant on every type table and on list_entry names the tenant of the node (the root while
-- there are none); a trigger fills it, so that moving rows into a tenant (--adopt) keeps it
-- right. The boundary of a session stays in the rights filter; the column serves uniqueness,
-- the key search of imports and, later, the figures over tenants (PL48). Type tables from
-- before get the column here; their indexes are rebuilt by schema.Apply.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION tenant_of(path ltree) RETURNS ltree LANGUAGE sql STABLE AS $$
    SELECT CASE
        WHEN nlevel(path) >= 2 AND EXISTS (SELECT 1 FROM org_node WHERE org_node.path = subltree(path, 0, 2) AND org_node.tenant)
        THEN subltree(path, 0, 2)
        ELSE 'root'::ltree
    END
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION set_tenant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.tenant := tenant_of(NEW.org_path);
    RETURN NEW;
END
$$;
-- +goose StatementEnd

ALTER TABLE list_entry ADD COLUMN tenant ltree NOT NULL DEFAULT 'root';
UPDATE list_entry SET tenant = tenant_of(org_path);
CREATE TRIGGER list_entry_tenant BEFORE INSERT OR UPDATE OF org_path ON list_entry
    FOR EACH ROW EXECUTE FUNCTION set_tenant();
DROP INDEX list_entry_key_key;
CREATE UNIQUE INDEX list_entry_key_key ON list_entry (tenant, list, key) WHERE deleted_at IS NULL;
DROP INDEX list_entry_source_key;
CREATE UNIQUE INDEX list_entry_source_key ON list_entry (tenant, list, source, external_key) WHERE source IS NOT NULL;

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT tablename FROM pg_tables WHERE schemaname = current_schema() AND tablename LIKE 'rec\_%' LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN tenant ltree NOT NULL DEFAULT ''root''', t);
        EXECUTE format('UPDATE %I SET tenant = tenant_of(org_path)', t);
        EXECUTE format('CREATE TRIGGER %I BEFORE INSERT OR UPDATE OF org_path ON %I FOR EACH ROW EXECUTE FUNCTION set_tenant()', t || '_tenant', t);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT tablename FROM pg_tables WHERE schemaname = current_schema() AND tablename LIKE 'rec\_%' LOOP
        EXECUTE format('DROP TRIGGER %I ON %I', t || '_tenant', t);
        EXECUTE format('ALTER TABLE %I DROP COLUMN tenant', t);
    END LOOP;
END
$$;
-- +goose StatementEnd
DROP INDEX list_entry_source_key;
CREATE UNIQUE INDEX list_entry_source_key ON list_entry (subltree(org_path, 0, least(nlevel(org_path), 2)), list, source, external_key)
    WHERE source IS NOT NULL;
DROP INDEX list_entry_key_key;
CREATE UNIQUE INDEX list_entry_key_key ON list_entry (subltree(org_path, 0, least(nlevel(org_path), 2)), list, key)
    WHERE deleted_at IS NULL;
DROP TRIGGER list_entry_tenant ON list_entry;
ALTER TABLE list_entry DROP COLUMN tenant;
DROP FUNCTION set_tenant();
DROP FUNCTION tenant_of(ltree);
