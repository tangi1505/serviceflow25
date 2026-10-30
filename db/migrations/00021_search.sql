-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Full-text search (Hauptplan E21, E101; PL26): the search column of the entries of all
-- select lists and the trigram index on their names. The type tables get theirs from the
-- schema builder.

-- +goose Up
CREATE INDEX list_entry_search_idx ON list_entry USING gin (search);
CREATE INDEX list_entry_name_trgm ON list_entry USING gin (name gin_trgm_ops);

-- +goose Down
DROP INDEX list_entry_name_trgm;
DROP INDEX list_entry_search_idx;
