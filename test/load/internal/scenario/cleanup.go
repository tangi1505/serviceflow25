// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package scenario

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Database is the database of the instance under measurement (--database of run and cleanup).
// With it the incidents a measurement created go in one transaction with SQL instead of one
// by one through the API (PL70, Pascal 2026-11-17: the instance in Prod size is there for the
// measurement only; through the API 265,000 incidents took 26 minutes). Empty: the API.
var Database string

// cleanupWindow is how far back the cleanup looks for incidents of LZ4: the index on
// created_at finds them without reading all 18 million.
const cleanupWindow = "7 days"

// cleanupCreatedSQL removes the incidents of LZ4 with everything that hangs on them: the rows
// of every table that names a record by type_name and record_id (roles, clocks, history,
// timeline, events, inbox), the jobs still waiting for them, and the incidents. No event tells
// of it, so the days of the day values (PL69) are marked for the run of the minute by hand.
func cleanupCreatedSQL(ctx context.Context, dbURL string) (int, error) {
	db, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return 0, fmt.Errorf("cleanup: %w", err)
	}
	defer db.Close(ctx)
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	table := pgx.Identifier{"rec_" + strings.ReplaceAll(incidentType, ".", "_")}.Sanitize()
	exec := func(sql string, args ...any) error {
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			return fmt.Errorf("cleanup: %s: %w", strings.SplitN(sql, "\n", 2)[0], err)
		}
		return nil
	}
	if err := exec(`CREATE TEMP TABLE gone ON COMMIT DROP AS
		SELECT id, created_at FROM `+table+`
		WHERE created_at >= now() - $1::interval AND title = $2 AND source IS NULL`, cleanupWindow, prodCreateTitle); err != nil {
		return 0, err
	}
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM gone").Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, tx.Commit(ctx)
	}
	for _, sql := range []string{
		"CREATE UNIQUE INDEX ON gone (id)",
		"ANALYZE gone",
		// The jobs that wait for these incidents: their rules, the distribution of their
		// events and the due times of their clocks. The kinds are those of the server.
		`CREATE TEMP TABLE gone_job ON COMMIT DROP AS
			SELECT c.id::text AS ref, 'sla.due' AS kind FROM clock c JOIN gone g ON g.id = c.record_id
			UNION ALL SELECT e.position::text, 'notify.fanout' FROM event e JOIN gone g ON g.id = e.record_id WHERE e.type_name = '` + incidentType + `'
			UNION ALL SELECT g.id::text, 'rules.apply' FROM gone g`,
		"CREATE INDEX ON gone_job (kind, ref)",
		"ANALYZE gone_job",
		`DELETE FROM river_job j USING gone_job g
			WHERE j.state IN ('available', 'scheduled', 'retryable') AND j.kind = g.kind
			AND g.ref = CASE j.kind WHEN 'sla.due' THEN j.args->>'clock' WHEN 'notify.fanout' THEN j.args->>'position' ELSE j.args->>'record' END`,
	} {
		if err := exec(sql); err != nil {
			return 0, err
		}
	}
	// Every table that names a record by type and id, found in the schema so that a new one
	// is not forgotten; a partition goes through its parent.
	rows, err := tx.Query(ctx, `SELECT c.table_name FROM information_schema.columns c
		JOIN information_schema.columns d USING (table_schema, table_name)
		WHERE c.table_schema = 'public' AND c.column_name = 'record_id' AND d.column_name = 'type_name'
		AND NOT EXISTS (SELECT 1 FROM pg_inherits i WHERE i.inhrelid = ('public.' || quote_ident(c.table_name))::regclass)
		ORDER BY 1`)
	if err != nil {
		return 0, fmt.Errorf("cleanup: tables: %w", err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, fmt.Errorf("cleanup: tables: %w", err)
	}
	for _, t := range tables {
		if err := exec("DELETE FROM "+pgx.Identifier{t}.Sanitize()+" t USING gone g WHERE t.type_name = $1 AND t.record_id = g.id", incidentType); err != nil {
			return 0, err
		}
	}
	if err := exec("DELETE FROM " + table + " t USING gone g WHERE t.id = g.id"); err != nil {
		return 0, err
	}
	// The day values count these incidents until their days are counted again.
	var days bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass('public.day_value_dirty') IS NOT NULL").Scan(&days); err != nil {
		return 0, err
	}
	if days {
		if err := exec(`INSERT INTO day_value_dirty (type_name, date_field, day)
			SELECT s.type_name, s.date_field, d.day FROM day_value_state s,
				(SELECT DISTINCT (created_at AT TIME ZONE 'Europe/Berlin')::date AS day FROM gone
				 UNION SELECT (now() AT TIME ZONE 'Europe/Berlin')::date) d
			WHERE s.type_name = $1 ON CONFLICT DO NOTHING`, incidentType); err != nil {
			return 0, err
		}
	}
	return n, tx.Commit(ctx)
}
