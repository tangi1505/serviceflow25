-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- A change that waits for approval (SD17, SD-Plan N22): a type may hold changes of some fields
-- while a condition holds (a published article); the change is kept at the approval and
-- written as a new version of the record once the approval is given. Without a change the
-- column is NULL, as for every approval before.

-- +goose Up
ALTER TABLE approval ADD COLUMN changes jsonb;

-- +goose Down
ALTER TABLE approval DROP COLUMN changes;
