-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Jobs (Hauptplan E19, PL23): River keeps its own tables (river_job, river_queue, ...), which
-- `itsm migrate` applies through River's migrator after these. job_schedule stores what an
-- administrator changed on a periodic job; the defaults live in the code (G183).

-- +goose Up
CREATE TABLE job_schedule (
    name       text PRIMARY KEY,
    schedule   text NOT NULL,
    enabled    boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid
);

-- +goose Down
DROP TABLE job_schedule;
