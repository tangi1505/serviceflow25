// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/query"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/rules/engine"
	"group24.de/itsm/internal/schema"
	"group24.de/itsm/internal/seclog"
)

// controlSetup is an installation with a database per tenant (PL47b): the control database
// and the databases of the tenants root.ta and root.tb, each migrated, with the packages core
// and probe. The definitions are central; the tables and the select lists are in each tenant.
type controlSetup struct {
	control, a, b *pgxpool.Pool
	rt            *db.Router
}

func newControlSetup(t *testing.T) controlSetup {
	t.Helper()
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	open := func(name string) *pgxpool.Pool {
		url := dbtest.Isolated(t, base, name)
		if err := db.Migrate(ctx, url, quiet); err != nil {
			t.Fatal(err)
		}
		pool, err := db.Open(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	s := controlSetup{control: open("itsmtest_ctl"), a: open("itsmtest_ctl_a"), b: open("itsmtest_ctl_b")}
	s.rt = db.NewRouter(s.control, map[string]*pgxpool.Pool{"root.ta": s.a, "root.tb": s.b},
		func(ctx context.Context) string { return authz.From(ctx).CurrentTenant() })
	var types []meta.Type
	for _, dir := range []string{"../../packages/core", "../../packages/probe"} {
		p, err := meta.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := meta.Store(ctx, s.control, p); err != nil {
			t.Fatal(err)
		}
		for _, pool := range s.rt.Pools() {
			if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
				t.Fatal(err)
			}
		}
		types = append(types, p.Types...)
	}
	// The tenants: in the directory of the control database, and each in its own database
	// (what itsm tenant create does with PL47d).
	for path, pool := range map[string]*pgxpool.Pool{"root.ta": s.a, "root.tb": s.b} {
		name := strings.TrimPrefix(path, "root.")
		for _, p := range []*pgxpool.Pool{s.control, pool} {
			if _, err := queries.New(p).InsertOrgNode(ctx, queries.InsertOrgNodeParams{Path: path, Name: name, Label: []byte(`{"de": "` + name + `"}`), Tenant: true}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := records.New(s.rt).SeedListsInto(authz.WithAccess(ctx, authz.SystemIn(path)), types, path); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// count runs a count in one database.
func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The acceptance of PL47b: with a database per tenant, accounts of both tenants and an
// administrator of the installation sign in, each finding the database of the account; the
// tenants' data stay in their databases; the administrator is mirrored into the tenant and
// may be named at a record there; a rule of the installation acts in both tenants; usernames
// stay unique across the databases; a role in use is not deleted; the security log merges the
// entries of the tenant and of the installation.
func TestControlDatabaseOverHTTP(t *testing.T) {
	s := newControlSetup(t)
	addr := os.Getenv("ITSM_TEST_VALKEY_ADDR")
	if addr == "" {
		t.Skip("ITSM_TEST_VALKEY_ADDR not set")
	}
	ctx := t.Context()
	kv, err := cache.Open(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(kv.Close)

	a := auth.New(s.rt, kv, quiet)
	az := authz.New(s.rt, nil, quiet)
	recs := records.New(s.rt)
	orgSvc := org.New(s.rt)
	rules := engine.New(s.rt, recs, nil, nil, quiet)
	recs.Before = rules.Before
	an := authn.New(s.rt, kv, a, az, recs, orgSvc, quiet, authn.Options{})
	h := NewRouter(Options{Logger: quiet, Version: "test", API: true, Records: recs, Auth: a, Org: orgSvc, Authz: az, Authn: an,
		Groups: query.NewGroups(s.rt), Rules: rules, SecLog: seclog.New(s.rt, orgSvc.Setting, quiet), KV: kv})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Accounts: the administrator of the installation centrally, with the role admin at the
	// root; anna in A and ben in B, each with the role admin at the tenant.
	person := func(tenant, name string) uuid.UUID {
		t.Helper()
		in := authz.WithAccess(ctx, authz.SystemIn(tenant))
		r, err := recs.Create(in, "core.person", map[string]any{"last_name": name}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return uuid.MustParse(r["id"].(string))
	}
	account := func(tenant, name string) (uuid.UUID, uuid.UUID) {
		t.Helper()
		acc, err := a.CreateAccountIn(ctx, tenant, name, name, testPassword, nil)
		if err != nil {
			t.Fatal(err)
		}
		node := tenant
		if node == "" {
			node = "root"
		}
		pid := person(node, name)
		if err := az.LinkPerson(ctx, acc.ID, &pid); err != nil {
			t.Fatal(err)
		}
		if _, err := az.Assign(authz.WithAccess(ctx, authz.SystemIn(node)), "admin", &pid, nil, node, nil); err != nil {
			t.Fatal(err)
		}
		return acc.ID, pid
	}
	adminID, adminPerson := account("", "ctl_admin")
	annaID, _ := account("root.ta", "ctl_anna")
	account("root.tb", "ctl_ben")
	az.Invalidate(ctx)

	// Every account lives in its database; the directory knows them all.
	for _, c := range []struct {
		name          string
		control, a, b int
	}{{"ctl_admin", 1, 0, 0}, {"ctl_anna", 0, 1, 0}, {"ctl_ben", 0, 0, 1}} {
		got := []int{count(t, s.control, "SELECT count(*) FROM account WHERE username = $1", c.name),
			count(t, s.a, "SELECT count(*) FROM account WHERE username = $1", c.name),
			count(t, s.b, "SELECT count(*) FROM account WHERE username = $1", c.name)}
		if got[0] != c.control || got[1] != c.a || got[2] != c.b || count(t, s.control, "SELECT count(*) FROM account_home WHERE username = $1", c.name) != 1 {
			t.Fatalf("account %s in control, A, B: %v", c.name, got)
		}
	}
	// A username is unique across the databases (E138).
	if _, err := a.CreateAccountIn(ctx, "root.tb", "ctl_anna", "x", testPassword, nil); err == nil {
		t.Fatal("ctl_anna twice")
	}
	if count(t, s.b, "SELECT count(*) FROM account WHERE username = 'ctl_anna'") != 0 {
		t.Fatal("the refused account was written")
	}

	login := func(name string) *testAPI {
		t.Helper()
		r := call(t, &testAPI{Server: srv}, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "`+testPassword+`"}`, nil)
		if r.code != 200 {
			t.Fatalf("login %s: %d %s", name, r.code, r.raw)
		}
		return &testAPI{Server: srv, cookie: sessionCookieOf(t, r.hdr)}
	}
	anna, ben, admin := login("ctl_anna"), login("ctl_ben"), login("ctl_admin")
	if r := call(t, anna, http.MethodGet, "/api/v1/auth/me", "", nil); r.body["tenant"] != "root.ta" {
		t.Fatalf("me of anna: %s", r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/auth/me", "", nil); r.body["tenant"] != "root.ta" || !strings.Contains(r.raw, "root.tb") {
		t.Fatalf("me of the administrator: %s", r.raw)
	}

	// The administrator entered A: account and person are mirrored there under their ids.
	if count(t, s.a, "SELECT count(*) FROM account WHERE id = $1 AND password_hash = '' AND person_id = $2", adminID, adminPerson) != 1 ||
		count(t, s.a, "SELECT count(*) FROM rec_core_person WHERE id = $1 AND org_path = 'root.ta'", adminPerson) != 1 {
		t.Fatal("no mirror of the administrator in A")
	}
	if count(t, s.b, "SELECT count(*) FROM account WHERE id = $1", adminID) != 0 {
		t.Fatal("a mirror in B before entering it")
	}

	// A rule of the installation is central and acts in both tenants.
	if r := call(t, admin, http.MethodPut, "/api/v1/rules/ctl_rule", `{"installation": true, "type_name": "probe.probe", "trigger": "create", "mode": "before", "actions": [{"set": {"solution": "Installation"}}]}`, nil); r.code != 201 {
		t.Fatalf("rule of the installation: %d %s", r.code, r.raw)
	}
	if count(t, s.control, "SELECT count(*) FROM rule WHERE name = 'ctl_rule' AND tenant IS NULL") != 1 || count(t, s.a, "SELECT count(*) FROM rule") != 0 {
		t.Fatal("the rule of the installation is not central")
	}

	// Records stay in their tenant's database; the rule set the solution in both.
	for _, c := range []struct {
		api  *testAPI
		pool *pgxpool.Pool
		node string
	}{{anna, s.a, "root.ta"}, {ben, s.b, "root.tb"}} {
		r := call(t, c.api, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "Vorgang"}`, nil)
		if r.code != 201 || r.body["org_path"] != c.node || r.body["solution"] != "Installation" {
			t.Fatalf("record in %s: %d %s", c.node, r.code, r.raw)
		}
		if count(t, c.pool, "SELECT count(*) FROM rec_probe_probe WHERE id = $1", r.body["id"]) != 1 ||
			count(t, s.control, "SELECT count(*) FROM rec_probe_probe") != 0 {
			t.Fatalf("record of %s not in its database", c.node)
		}
	}

	// The administrator names himself as assignee at a record in A: the mirror makes the
	// reference valid there.
	body := `{"title": "Für den Admin", "actors": {"assigned": [{"type": "core.person", "id": "` + adminPerson.String() + `"}]}}`
	if r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", body, nil); r.code != 201 || !strings.Contains(r.raw, adminPerson.String()) {
		t.Fatalf("administrator as assignee in A: %d %s", r.code, r.raw)
	}
	// Switching to B mirrors him there.
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/context", `{"tenant": "root.tb"}`, nil); r.code != 200 {
		t.Fatalf("switch to B: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe", "", nil); r.code != 200 {
		t.Fatalf("list in B: %d %s", r.code, r.raw)
	}
	if count(t, s.b, "SELECT count(*) FROM account WHERE id = $1", adminID) != 1 {
		t.Fatal("no mirror of the administrator in B after the switch")
	}

	// A role assigned in A is not deleted; without its assignment it is.
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/context", `{"tenant": "root.ta"}`, nil); r.code != 200 {
		t.Fatalf("back to A: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "ctl_role"}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	annaPerson := ""
	if r := call(t, anna, http.MethodGet, "/api/v1/auth/me", "", nil); r.code == 200 {
		annaPerson, _ = r.body["person_id"].(string)
	}
	as := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "ctl_role", "person_id": "`+annaPerson+`", "org_path": "root.ta"}`, nil)
	if as.code != 201 || count(t, s.a, "SELECT count(*) FROM role_assignment WHERE id = $1", as.body["id"]) != 1 {
		t.Fatalf("assignment in A: %d %s", as.code, as.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/assignments", "", nil); !strings.Contains(r.raw, `"role":"ctl_role"`) {
		t.Fatalf("assignments without the role's name: %s", r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/roles/ctl_role", "", nil); r.code != 409 {
		t.Fatalf("deleting a role in use: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/assignments/"+as.body["id"].(string), "", nil); r.code != 204 {
		t.Fatalf("unassign: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/roles/ctl_role", "", nil); r.code != 204 {
		t.Fatalf("deleting the role: %d %s", r.code, r.raw)
	}

	// The security log: the administrator in A sees the logins of A and of the installation,
	// merged from two databases; anna's login is in A, the administrator's centrally.
	if count(t, s.a, "SELECT count(*) FROM security_event WHERE username = 'ctl_anna' AND kind = 'login'") != 1 ||
		count(t, s.control, "SELECT count(*) FROM security_event WHERE username = 'ctl_admin' AND kind = 'login'") != 1 {
		t.Fatal("logins not in their databases")
	}
	r := call(t, admin, http.MethodGet, "/api/v1/audit/auth?kind=login", "", nil)
	if r.code != 200 || !strings.Contains(r.raw, `"ctl_anna"`) || !strings.Contains(r.raw, `"ctl_admin"`) || strings.Contains(r.raw, `"ctl_ben"`) {
		t.Fatalf("security log of the administrator in A: %d %s", r.code, r.raw)
	}
	if count(t, s.control, "SELECT count(*) FROM security_event WHERE id < (1::bigint << 50)") != 0 {
		t.Fatal("security event ids are not ordered by time")
	}
	_ = annaID
}
