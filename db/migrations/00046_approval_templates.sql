-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Templates of approvals and reminders (WF7, WF-Plan E18; Inventar G45, G67). An approval
-- asked from a template (core.approval_template) names it and takes over its number of
-- working days between reminders (NULL: the setting approvals.remind_days of the record's
-- node). While a stage is open a job every minute reminds those who have not decided yet once
-- remind_at is reached, counts the reminders and sets the next moment; NULL means no
-- reminder (0 working days).

-- +goose Up
ALTER TABLE approval ADD COLUMN template_id uuid;
ALTER TABLE approval ADD COLUMN remind_days integer CHECK (remind_days >= 0);
ALTER TABLE approval_stage ADD COLUMN remind_at timestamptz;
ALTER TABLE approval_stage ADD COLUMN reminded_at timestamptz;
ALTER TABLE approval_stage ADD COLUMN reminders integer NOT NULL DEFAULT 0;
-- The job looks for the stages due.
CREATE INDEX approval_stage_remind ON approval_stage (remind_at) WHERE state = 'open' AND remind_at IS NOT NULL;

-- +goose Down
DROP INDEX approval_stage_remind;
ALTER TABLE approval_stage DROP COLUMN reminders;
ALTER TABLE approval_stage DROP COLUMN reminded_at;
ALTER TABLE approval_stage DROP COLUMN remind_at;
ALTER TABLE approval DROP COLUMN remind_days;
ALTER TABLE approval DROP COLUMN template_id;
