-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Every change of the definitions (package install or update, the type editor) tells the
-- instances on the channel itsm_definitions, so that they load the types anew (PL53): the
-- types are kept in memory instead of being read for every call.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION definition_notify() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('itsm_definitions', '');
    RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER definition_notify AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON definition
    FOR EACH STATEMENT EXECUTE FUNCTION definition_notify();

-- +goose Down
DROP TRIGGER definition_notify ON definition;
DROP FUNCTION definition_notify();
