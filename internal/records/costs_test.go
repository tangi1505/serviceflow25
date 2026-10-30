// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

const costType = "coststest.job"

// setupCosts installs the package coststest: job with the traits history and costs, plain
// without costs. The hourly rate of the tree is 80.
func setupCosts(t *testing.T) (*Service, context.Context, *pgxpool.Pool) {
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
		"DROP TABLE IF EXISTS rec_coststest_job", "DROP TABLE IF EXISTS rec_coststest_plain",
		"DELETE FROM package WHERE name = 'coststest'",
		"DELETE FROM audit WHERE type_name LIKE 'coststest.%'",
		"DELETE FROM event WHERE type_name LIKE 'coststest.%'",
		"DELETE FROM cost WHERE type_name LIKE 'coststest.%'",
		"DELETE FROM schema_change WHERE type_name LIKE 'coststest.%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte("name: coststest\nversion: 0.1.0\nlabel: {de: K, en: C}\n")},
		"types/job.yaml":   {Data: []byte("name: job\nlabel: {de: A, en: J}\ntraits: [history, costs]\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
		"types/plain.yaml": {Data: []byte("name: plain\nlabel: {de: P, en: P}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
		t.Fatal(err)
	}
	s := New(pool)
	s.Settings = func(_ context.Context, path, key string) (json.RawMessage, string, error) {
		if key == SettingHourlyRate && strings.HasPrefix(path, "root") {
			return json.RawMessage("80"), "root", nil
		}
		return nil, "", nil
	}
	return s, ctx, pool
}

// The acceptance of PL20: three entries on a record give the sums on the record, also after
// one is removed; booking leaves the record's version, so an edit with If-Match still works.
func TestCostSumsFollowTheEntries(t *testing.T) {
	s, ctx, pool := setupCosts(t)
	rec, err := s.Create(ctx, costType, map[string]any{"title": "printer"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rec["time_spent"] != 0.0 || rec["cost_total"] != 0.0 {
		t.Fatalf("a new record starts at zero: %v", rec)
	}
	rid := id(t, rec)

	first, err := s.AddCost(ctx, costType, rid, map[string]any{"kind": "time", "minutes": json.Number("90"), "rate": 60.0, "spent_on": "2026-10-26"}, nil)
	if err != nil || first.Amount != 90 || *first.Minutes != 90 || *first.Rate != 60 || first.Version != 1 {
		t.Fatalf("time entry with rate: %+v %v", first, err)
	}
	second, err := s.AddCost(ctx, costType, rid, map[string]any{"kind": "time", "minutes": 30.0, "description": "on site"}, nil)
	if err != nil || *second.Rate != 80 || second.Amount != 40 || second.SpentOn != time.Now().Format(dateFormat) {
		t.Fatalf("time entry takes costs.hourly_rate and today: %+v %v", second, err)
	}
	third, err := s.AddCost(ctx, costType, rid, map[string]any{"kind": "material", "amount": 25.5}, nil)
	if err != nil || third.Amount != 25.5 || third.Minutes != nil || third.Rate != nil {
		t.Fatalf("material entry: %+v %v", third, err)
	}

	got, err := s.Get(ctx, costType, rid)
	if err != nil || got["time_spent"] != 120.0 || got["cost_total"] != 155.5 || got["version"] != int64(1) {
		t.Fatalf("sums after three entries: %v %v", got, err)
	}
	list, err := s.Costs(ctx, costType, rid)
	if err != nil || len(list.Entries) != 3 || list.TimeSpent != 120 || list.CostTotal != 155.5 || list.Currency != "EUR" ||
		list.Entries[0].ID != first.ID {
		t.Fatalf("list: %+v %v", list, err)
	}

	// Removing one entry takes it out of both sums.
	if err := s.RemoveCost(ctx, costType, rid, first.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Get(ctx, costType, rid); got["time_spent"] != 30.0 || got["cost_total"] != 65.5 {
		t.Fatalf("sums after removing the first entry: %v", got)
	}
	if err := s.RemoveCost(ctx, costType, rid, first.ID, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removing twice: %v", err)
	}

	// Changing an entry needs its version; the sums follow.
	if _, err := s.UpdateCost(ctx, costType, rid, third.ID, 2, map[string]any{"amount": 30.0}, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("change with a stale version: %v", err)
	}
	changed, err := s.UpdateCost(ctx, costType, rid, third.ID, 1, map[string]any{"amount": 30.0}, nil)
	if err != nil || changed.Version != 2 || changed.Amount != 30 {
		t.Fatalf("change: %+v %v", changed, err)
	}
	if changed, err = s.UpdateCost(ctx, costType, rid, second.ID, 1, map[string]any{"minutes": 45.0}, nil); err != nil || changed.Amount != 60 {
		t.Fatalf("a time entry's amount follows its minutes at its rate: %+v %v", changed, err)
	}
	if got, _ = s.Get(ctx, costType, rid); got["time_spent"] != 45.0 || got["cost_total"] != 90.0 || got["version"] != int64(1) {
		t.Fatalf("sums after the changes: %v", got)
	}

	// The record was never raised: an edit with the version the caller saw goes through.
	if upd, err := s.Update(ctx, costType, rid, 1, map[string]any{"title": "printer 2"}, nil); err != nil || upd["cost_total"] != 90.0 {
		t.Fatalf("edit after bookings: %v %v", upd, err)
	}

	// History: every write of an entry is a row with the entry, in the version it happened at.
	versions, err := s.Versions(ctx, costType, rid)
	if err != nil || len(versions) != 2 || strings.Join(versions[0].Actions, ",") != "create,update,cost_add,cost_remove,cost_change" ||
		strings.Join(versions[0].Changed, ",") != "title" {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	v1, err := s.Version(ctx, costType, rid, 1)
	if _, has := v1["cost_total"]; err != nil || has || v1["title"] != "printer" {
		t.Fatalf("a version has no sums: %v %v", v1, err)
	}

	// Purging the record removes its entries.
	if err := s.Trash(ctx, costType, rid, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCost(ctx, costType, rid, map[string]any{"kind": "fixed", "amount": 1.0}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("booking on a trashed record: %v", err)
	}
	if err := s.Purge(ctx, costType, rid, nil); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM cost WHERE record_id = $1", rid).Scan(&left); err != nil || left != 0 {
		t.Fatalf("entries after purge: %d %v", left, err)
	}
}

// Wrong input is refused field by field; the sums cannot be written; a type without the trait
// has no costs.
func TestCostValidation(t *testing.T) {
	s, ctx, _ := setupCosts(t)
	rec, err := s.Create(ctx, costType, map[string]any{"title": "x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rid := id(t, rec)
	cases := []struct {
		input map[string]any
		field string
		code  string
	}{
		{map[string]any{"minutes": 5.0}, "kind", "required"},
		{map[string]any{"kind": "travel"}, "kind", "choice"},
		{map[string]any{"kind": "time"}, "minutes", "required"},
		{map[string]any{"kind": "time", "minutes": 1.5}, "minutes", "type"},
		{map[string]any{"kind": "time", "minutes": 0.0}, "minutes", "min"},
		{map[string]any{"kind": "time", "minutes": 5.0, "amount": 3.0}, "amount", "read_only"},
		{map[string]any{"kind": "fixed"}, "amount", "required"},
		{map[string]any{"kind": "fixed", "amount": -1.0}, "amount", "min"},
		{map[string]any{"kind": "material", "amount": 1.0, "minutes": 5.0}, "minutes", "not_allowed"},
		{map[string]any{"kind": "fixed", "amount": 1.0, "rate": 5.0}, "rate", "not_allowed"},
		{map[string]any{"kind": "fixed", "amount": 1.0, "spent_on": "26.10.2026"}, "spent_on", "type"},
		{map[string]any{"kind": "fixed", "amount": 1.0, "person": uuid.NewString()}, "person", "reference"},
		{map[string]any{"kind": "fixed", "amount": 1.0, "hours": 2.0}, "hours", "unknown"},
	}
	for _, c := range cases {
		_, err := s.AddCost(ctx, costType, rid, c.input, nil)
		ve, ok := AsValidationError(err)
		if !ok || !hasProblem(ve, c.field, c.code) {
			t.Errorf("%v: want %s/%s, got %v", c.input, c.field, c.code, err)
		}
	}
	entry, err := s.AddCost(ctx, costType, rid, map[string]any{"kind": "fixed", "amount": 10.0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateCost(ctx, costType, rid, entry.ID, 1, map[string]any{"kind": "time"}, nil); !isProblem(err, "kind", "read_only") {
		t.Errorf("changing the kind: %v", err)
	}
	if _, err := s.Update(ctx, costType, rid, 1, map[string]any{"cost_total": 0.0}, nil); !isProblem(err, "cost_total", "read_only") {
		t.Errorf("writing a sum: %v", err)
	}
	if _, err := s.Create(ctx, costType, map[string]any{"title": "y", "time_spent": 5.0}, nil); !isProblem(err, "time_spent", "read_only") {
		t.Errorf("creating with a sum: %v", err)
	}
	plain, err := s.Create(ctx, "coststest.plain", map[string]any{"title": "z"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Costs(ctx, "coststest.plain", id(t, plain)); !errors.Is(err, ErrNotFound) {
		t.Errorf("costs of a type without the trait: %v", err)
	}
	if _, has := plain["cost_total"]; has {
		t.Errorf("a type without the trait has no sums: %v", plain)
	}
}

// Parallel bookings on one record lose nothing: the lock on the record's row makes every sum
// count the entries committed before it.
func TestCostSumsUnderParallelBookings(t *testing.T) {
	s, ctx, _ := setupCosts(t)
	rec, err := s.Create(ctx, costType, map[string]any{"title": "busy"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rid := id(t, rec)
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.AddCost(ctx, costType, rid, map[string]any{"kind": "time", "minutes": 3.0, "rate": 20.0}, nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Get(ctx, costType, rid)
	if err != nil || got["time_spent"] != float64(3*n) || got["cost_total"] != float64(n) {
		t.Fatalf("sums after %d parallel bookings: %v %v", n, got, err)
	}
}

func hasProblem(ve *ValidationError, field, code string) bool {
	for _, p := range ve.Problems {
		if p.Field == field && p.Code == code {
			return true
		}
	}
	return false
}

func isProblem(err error, field, code string) bool {
	ve, ok := AsValidationError(err)
	return ok && hasProblem(ve, field, code)
}
