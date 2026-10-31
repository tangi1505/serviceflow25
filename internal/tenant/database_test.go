// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package tenant_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/schema"
	"group24.de/itsm/internal/tenant"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

const password = "Mandant-Pruef1!"

// The acceptance of PL47d (Hauptplan E144): with a database per tenant the start is strict, a
// new tenant gets the empty database the operator made, a package update reaches every
// database, and a tenant's database can be backed up and restored on its own with the tools of
// PostgreSQL, after which itsm tenant check finds it fitting the control database, or says
// what is missing.
func TestTenantDatabase(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	controlURL := dbtest.Isolated(t, base, "itsmtest_pl47d")
	if err := db.Migrate(ctx, controlURL, quiet); err != nil {
		t.Fatal(err)
	}
	control, err := db.Open(ctx, controlURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(control.Close)
	install := func(rt *db.Router, p meta.Package) {
		t.Helper()
		if err := meta.Store(ctx, control, p); err != nil {
			t.Fatal(err)
		}
		for _, pool := range rt.Pools() {
			if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
				t.Fatal(err)
			}
		}
	}
	load := func(dir string) meta.Package {
		t.Helper()
		p, err := meta.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	install(db.Single(control), load("../../packages/core"))
	install(db.Single(control), load("../../packages/probe")) // ships select lists

	// The operator made the database of nord, empty, and set its URL.
	urls := map[string]string{"nord": dbtest.Isolated(t, base, "itsmtest_pl47d_nord")}
	urlOf := func(name string) string { return urls[name] }
	if _, err := tenant.CreateIn(ctx, control, "", tenant.Options{Name: "nord"}, quiet); !errors.Is(err, db.ErrNoDatabase) {
		t.Fatalf("without a URL: %v", err)
	}
	if _, err := tenant.CreateIn(ctx, control, urls["nord"], tenant.Options{Name: "nord", Adopt: true}, quiet); !errors.Is(err, tenant.ErrInvalid) {
		t.Fatalf("adopting into a database of its own: %v", err)
	}
	res, err := tenant.CreateIn(ctx, control, urls["nord"], tenant.Options{Name: "nord", Label: map[string]string{"de": "Nord"}}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tenant.Path != "root.nord" || res.Seeded == 0 {
		t.Fatalf("created: %+v", res)
	}
	if _, err := tenant.CreateIn(ctx, control, urls["nord"], tenant.Options{Name: "nord"}, quiet); !errors.Is(err, org.ErrExists) {
		t.Fatalf("twice: %v", err)
	}

	// The start is strict: every tenant of the directory needs its database at the migration of
	// the control database.
	if _, err := db.OpenTenants(ctx, control, func(string) string { return "" }); !errors.Is(err, db.ErrNoDatabase) ||
		!strings.Contains(err.Error(), "ITSM_TENANT_NORD_DATABASE_URL") {
		t.Fatalf("a tenant without its URL must stop the start: %v", err)
	}
	pools, err := db.OpenTenants(ctx, control, urlOf)
	if err != nil {
		t.Fatal(err)
	}
	nord := pools["root.nord"]
	t.Cleanup(nord.Close)
	rt := db.NewRouter(control, pools, func(ctx context.Context) string { return authz.From(ctx).CurrentTenant() })
	count := func(pool *pgxpool.Pool, sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count(nord, "SELECT count(*) FROM list_entry WHERE tenant = 'root.nord'") != res.Seeded ||
		count(control, "SELECT count(*) FROM list_entry WHERE tenant = 'root.nord'") != 0 {
		t.Fatal("the select lists of nord belong into its database")
	}

	// A package installed afterwards reaches the database of the tenant too.
	later, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte("name: pl47dtest\nversion: 0.1.0\nlabel: {de: P, en: P}\n")},
		"types/thing.yaml": {Data: []byte("name: thing\nlabel: {de: D, en: T}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	install(rt, later)
	if count(nord, "SELECT count(*) FROM information_schema.tables WHERE table_name = 'rec_pl47dtest_thing'") != 1 {
		t.Fatal("the package update did not reach the tenant's database")
	}

	// Data of nord: an account and a record, both in its database.
	a := auth.New(rt, nil, quiet)
	if _, err := a.CreateAccountIn(ctx, "root.nord", "pl47d_anna", "Anna", password, nil); err != nil {
		t.Fatal(err)
	}
	inNord := authz.WithAccess(ctx, authz.SystemIn("root.nord"))
	if _, err := records.New(rt).Create(inNord, "probe.probe", map[string]any{"title": "gesichert"}, nil); err != nil {
		t.Fatal(err)
	}
	if f, err := tenant.Check(ctx, control, nord, "root.nord"); err != nil || len(f) != 0 {
		t.Fatalf("check after create: %v %v", f, err)
	}

	// Backup with pg_dump, then a change after it, then restore into a new database.
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump not installed: the tool container has it")
	}
	dump := filepath.Join(t.TempDir(), "nord.dump")
	if out, err := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-owner", "--file="+dump, urls["nord"]).CombinedOutput(); err != nil {
		t.Fatalf("pg_dump: %v %s", err, out)
	}
	if _, err := a.CreateAccountIn(ctx, "root.nord", "pl47d_ben", "Ben", password, nil); err != nil {
		t.Fatal(err)
	}
	restored := dbtest.Isolated(t, base, "itsmtest_pl47d_nord_restored")
	if out, err := exec.CommandContext(ctx, "pg_restore", "--no-owner", "--exit-on-error", "--dbname="+restored, dump).CombinedOutput(); err != nil {
		t.Fatalf("pg_restore: %v %s", err, out)
	}
	back, err := db.Open(ctx, restored)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(back.Close)
	if count(back, "SELECT count(*) FROM rec_probe_probe WHERE title = 'gesichert'") != 1 ||
		count(back, "SELECT count(*) FROM account WHERE username = 'pl47d_anna'") != 1 {
		t.Fatal("the restored database lacks what was backed up")
	}
	// The check names what the backup cannot know: ben came after it.
	f, err := tenant.Check(ctx, control, back, "root.nord")
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 1 || f[0].What != "account" || !strings.Contains(f[0].Detail, "pl47d_ben") {
		t.Fatalf("check of the restored database: %+v", f)
	}
	// The database of nord itself fits.
	if f, err := tenant.Check(ctx, control, nord, "root.nord"); err != nil || len(f) != 0 {
		t.Fatalf("check of nord: %v %v", f, err)
	}
}
