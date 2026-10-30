-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Escalation levels (WF10; Inventar G66, G168; Hauptplan KF5, KF7): a level is a rule with the
-- trigger clock, the SLA it watches and the share of its target in percent (1 to 1000, above
-- 100 after the deadline). A clock reaches each level once; clock.levels keeps the names of
-- the rules it reached. Reaching a level leaves an audit row and an event "escalation", from
-- which the rules consumer queues the job of that one rule.

-- +goose Up
ALTER TABLE rule ADD COLUMN sla uuid;
ALTER TABLE rule ADD COLUMN percent integer CHECK (percent BETWEEN 1 AND 1000);
ALTER TABLE rule DROP CONSTRAINT rule_trigger_check;
ALTER TABLE rule ADD CONSTRAINT rule_trigger_check CHECK (trigger IN ('create', 'update', 'time', 'clock'));
ALTER TABLE rule ADD CONSTRAINT rule_clock_check
    CHECK ((trigger = 'clock') = (sla IS NOT NULL AND percent IS NOT NULL) AND (trigger <> 'clock' OR mode = 'after'));
CREATE INDEX rule_sla_idx ON rule (sla) WHERE trigger = 'clock';

ALTER TABLE clock ADD COLUMN levels text[] NOT NULL DEFAULT '{}';

ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach',
                      'cost_add', 'cost_change', 'cost_remove', 'notify', 'breach', 'escalation'));

-- +goose Down
DELETE FROM audit WHERE action = 'escalation';
ALTER TABLE audit DROP CONSTRAINT audit_action_check;
ALTER TABLE audit ADD CONSTRAINT audit_action_check
    CHECK (action IN ('create', 'update', 'trash', 'restore', 'purge', 'transition', 'relate', 'unrelate', 'attach', 'detach',
                      'cost_add', 'cost_change', 'cost_remove', 'notify', 'breach'));
ALTER TABLE clock DROP COLUMN levels;
DELETE FROM rule WHERE trigger = 'clock';
DROP INDEX rule_sla_idx;
ALTER TABLE rule DROP CONSTRAINT rule_clock_check;
ALTER TABLE rule DROP CONSTRAINT rule_trigger_check;
ALTER TABLE rule ADD CONSTRAINT rule_trigger_check CHECK (trigger IN ('create', 'update', 'time'));
ALTER TABLE rule DROP COLUMN percent;
ALTER TABLE rule DROP COLUMN sla;
