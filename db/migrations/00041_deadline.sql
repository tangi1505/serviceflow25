-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The deadline of a record for its list (UI34, UI-Plan E37, Hauptplan E156): every type table
-- and list_entry carry the next deadline of the record's running SLA clocks, the moment that
-- clock has used three quarters of its target, and whether the record only has paused clocks.
-- internal/sla keeps them in the transaction that moves the clocks; the list sorts and filters
-- by them with an index. They are no fields: no version, no history, not in row_hash.
--
-- Rows from before are filled from the clocks; the three-quarter point as the deadline less a
-- quarter of the target, which is exact for clocks round the clock and early for clocks with a
-- calendar, until the next write of the record or the next job of its clock. New type tables
-- get the columns from schema.Apply.

-- +goose Up
ALTER TABLE list_entry
    ADD COLUMN sla_due_at timestamptz,
    ADD COLUMN sla_warn_at timestamptz,
    ADD COLUMN sla_paused boolean NOT NULL DEFAULT false;

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT tablename FROM pg_tables WHERE schemaname = current_schema() AND tablename LIKE 'rec\_%' LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN sla_due_at timestamptz, ADD COLUMN sla_warn_at timestamptz, '
            'ADD COLUMN sla_paused boolean NOT NULL DEFAULT false', t);
        EXECUTE format('CREATE INDEX %I ON %I (sla_due_at, id)', t || '_sla_due_idx', t);
        EXECUTE format('UPDATE %I r SET sla_due_at = d.due, sla_warn_at = d.warn, sla_paused = d.paused FROM ('
            'SELECT record_id, min(due_at) FILTER (WHERE state = ''running'') AS due, '
            '(array_agg(due_at - make_interval(secs => target_seconds / 4.0) ORDER BY due_at) FILTER (WHERE state = ''running''))[1] AS warn, '
            'bool_or(state = ''paused'') AND NOT bool_or(state = ''running'') AS paused '
            'FROM clock GROUP BY record_id) d WHERE r.id = d.record_id', t);
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
        EXECUTE format('ALTER TABLE %I DROP COLUMN sla_due_at, DROP COLUMN sla_warn_at, DROP COLUMN sla_paused', t);
    END LOOP;
END
$$;
-- +goose StatementEnd
ALTER TABLE list_entry DROP COLUMN sla_due_at, DROP COLUMN sla_warn_at, DROP COLUMN sla_paused;
