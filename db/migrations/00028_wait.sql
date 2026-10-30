-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Reasons to wait (WF11; Inventar G54): reminders and a solution after some days are
-- escalation levels of an SLA that holds while a record waits. Such levels name a working time
-- in minutes instead of a share of the target ("after 2 days"); a level has exactly one of the
-- two.

-- +goose Up
ALTER TABLE rule ADD COLUMN minutes integer CHECK (minutes > 0);
ALTER TABLE rule DROP CONSTRAINT rule_clock_check;
ALTER TABLE rule ADD CONSTRAINT rule_clock_check
    CHECK ((trigger = 'clock') = (sla IS NOT NULL AND ((percent IS NOT NULL) <> (minutes IS NOT NULL)))
           AND (trigger <> 'clock' OR mode = 'after'));

-- +goose Down
DELETE FROM rule WHERE trigger = 'clock' AND minutes IS NOT NULL;
ALTER TABLE rule DROP CONSTRAINT rule_clock_check;
ALTER TABLE rule ADD CONSTRAINT rule_clock_check
    CHECK ((trigger = 'clock') = (sla IS NOT NULL AND percent IS NOT NULL) AND (trigger <> 'clock' OR mode = 'after'));
ALTER TABLE rule DROP COLUMN minutes;
