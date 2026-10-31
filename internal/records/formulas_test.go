// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

const ticketType = "formulatest.ticket"

// ticketYAML is the type with its formulas; priority is passed in, so that a test can change it.
func ticketYAML(priority string) string {
	return `name: ticket
label: {de: Ticket, en: Ticket}
traits: [history]
fields:
  - {name: title, type: text, label: {de: T, en: T}}
  - {name: impact, type: number, label: {de: I, en: I}}
  - {name: urgency, type: number, label: {de: U, en: U}}
  - {name: priority, type: number, label: {de: P, en: P}, formula: '` + priority + `'}
  - name: level
    type: choice
    label: {de: L, en: L}
    options: [{value: low, label: {de: N, en: L}}, {value: high, label: {de: H, en: H}}]
    formula: 'priority >= 6.0 ? "high" : (priority > 0.0 ? "low" : "none")'
  - {name: opened_on, type: date, label: {de: O, en: O}}
  - {name: due_on, type: date, label: {de: F, en: D}, formula: 'opened_on + duration("72h")'}
`
}

func installTickets(t *testing.T, ctx context.Context, pool *pgxpool.Pool, priority string) {
	t.Helper()
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":      {Data: []byte("name: formulatest\nversion: 0.1.0\nlabel: {de: F, en: F}\n")},
		"types/ticket.yaml": {Data: []byte(ticketYAML(priority))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Validate(p, nil); err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
		t.Fatal(err)
	}
}

func setupFormulas(t *testing.T) (*Service, context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, sql := range []string{
		"DROP TABLE IF EXISTS rec_formulatest_ticket",
		"DELETE FROM package WHERE name = 'formulatest'",
		"DELETE FROM audit WHERE type_name LIKE 'formulatest.%'",
		"DELETE FROM schema_change WHERE type_name LIKE 'formulatest.%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	installTickets(t, ctx, pool, "impact * urgency")
	return New(db.Single(pool)), ctx, pool
}

// The acceptance of PL22: priority from impact and urgency is set on create and update, and
// so are the fields calculated from it; an empty input gives an empty result.
func TestCalculatedFields(t *testing.T) {
	s, ctx, _ := setupFormulas(t)
	rec, err := s.Create(ctx, ticketType, map[string]any{"title": "a", "impact": 2.0, "urgency": 3.0, "opened_on": "2026-10-27"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rec["priority"] != 6.0 || rec["level"] != "high" || rec["due_on"] != "2026-10-30" {
		t.Fatalf("create: %v", rec)
	}
	rid := id(t, rec)
	upd, err := s.Update(ctx, ticketType, rid, 1, map[string]any{"urgency": 1.0}, nil)
	if err != nil || upd["priority"] != 2.0 || upd["level"] != "low" || upd["version"] != int64(2) {
		t.Fatalf("update urgency: %v %v", upd, err)
	}
	if upd, err = s.Update(ctx, ticketType, rid, 2, map[string]any{"impact": nil, "opened_on": nil}, nil); err != nil ||
		upd["priority"] != nil || upd["level"] != nil || upd["due_on"] != nil {
		t.Fatalf("empty impact: %v %v", upd, err)
	}
	// The history has the calculated values like any change.
	versions, err := s.Versions(ctx, ticketType, rid)
	if err != nil || strings.Join(versions[1].Changed, ",") != "urgency,priority,level" {
		t.Fatalf("versions: %+v %v", versions, err)
	}

	// Calculated fields cannot be written; a result outside the options refuses the write.
	if _, err := s.Update(ctx, ticketType, rid, 3, map[string]any{"priority": 1.0}, nil); !isProblem(err, "priority", "read_only") {
		t.Fatalf("writing a calculated field: %v", err)
	}
	if _, err := s.Create(ctx, ticketType, map[string]any{"impact": 0.0, "urgency": 3.0}, nil); !isProblem(err, "level", "formula") {
		t.Fatalf("a result outside the options: %v", err)
	}
}

// A changed formula is applied to the records that exist by the job records.recompute,
// without a new version and without audit rows.
func TestRecomputeAfterChangedFormula(t *testing.T) {
	s, ctx, pool := setupFormulas(t)
	var ids []string
	for i := range 3 {
		rec, err := s.Create(ctx, ticketType, map[string]any{"impact": float64(i + 1), "urgency": 2.0}, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec["id"].(string))
	}
	empty, err := s.Create(ctx, ticketType, map[string]any{"title": "no input"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var auditBefore int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE type_name = $1", ticketType).Scan(&auditBefore); err != nil {
		t.Fatal(err)
	}

	installTickets(t, ctx, pool, "impact + urgency")
	changed, failed, err := s.Recompute(ctx, ticketType)
	if err != nil || changed != 2 || failed != 0 {
		// impact 2, urgency 2 gives 4 either way; the record without input stays empty.
		t.Fatalf("recompute: changed %d, failed %d, %v", changed, failed, err)
	}
	for i, want := range []float64{3, 4, 5} {
		rec, err := s.Get(ctx, ticketType, id(t, Record{"id": ids[i]}))
		if err != nil || rec["priority"] != want || rec["version"] != int64(1) {
			t.Fatalf("record %d after recompute: %v %v", i, rec, err)
		}
	}
	if rec, _ := s.Get(ctx, ticketType, id(t, empty)); rec["priority"] != nil {
		t.Fatalf("record without input: %v", rec)
	}
	var auditAfter int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE type_name = $1", ticketType).Scan(&auditAfter); err != nil || auditAfter != auditBefore {
		t.Fatalf("audit rows: %d before, %d after", auditBefore, auditAfter)
	}
}
