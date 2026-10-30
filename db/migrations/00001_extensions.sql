-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Extensions the core needs: ltree for the organisation tree (architektur.md §5),
-- pg_trgm for fault-tolerant name search (architektur.md §11, LZ3).

-- +goose Up
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- +goose Down
DROP EXTENSION IF EXISTS pg_trgm;
DROP EXTENSION IF EXISTS ltree;
