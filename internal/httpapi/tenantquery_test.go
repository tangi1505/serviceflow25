// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/tenant"
)

// The acceptance of PL48 (Hauptplan E137, E146), in both operating modes: an administrator of
// the installation with the right report_tenants sees the records of each status for A and B
// in one table, the tenant as a column; the built-in admin (every action) and an administrator
// of a tenant get 403; a list comes with the tenant too; an unknown tenant is skipped and
// named; there is no paging; each tenant has the evaluation in its security log.
func TestTenantQueryOverHTTP(t *testing.T) {
	t.Run("shared", func(t *testing.T) { tenantQueryCase(t, false) })
	t.Run("database", func(t *testing.T) { tenantQueryCase(t, true) })
}

func tenantQueryCase(t *testing.T, separate bool) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx := t.Context()
	var admin *testAPI
	if separate {
		t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_pl48_ctl"))
		urls := map[string]string{"root.ta": dbtest.Isolated(t, base, "itsmtest_api_pl48_ta"), "root.tb": dbtest.Isolated(t, base, "itsmtest_api_pl48_tb")}
		admin = apiServerWith(t, urls)
		tenantsInDatabases(t, admin, urls)
	} else {
		t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_pl48"))
		admin = apiServer(t)
		for _, name := range []string{"ta", "tb"} {
			if _, err := tenant.Create(ctx, admin.pool, tenant.Options{Name: name}); err != nil {
				t.Fatal(err)
			}
		}
		admin.authz.Invalidate(ctx)
	}

	// Records: two in A, one in B, made by the administrator in each tenant.
	for path, n := range map[string]int{"root.ta": 2, "root.tb": 1} {
		if r := call(t, admin, http.MethodPost, "/api/v1/auth/context", `{"tenant": "`+path+`"}`, nil); r.code != 200 {
			t.Fatalf("switch to %s: %d %s", path, r.code, r.raw)
		}
		for i := range n {
			if r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", fmt.Sprintf(`{"title": "pl48 %s %d"}`, path, i), nil); r.code != 201 {
				t.Fatalf("record in %s: %d %s", path, r.code, r.raw)
			}
		}
	}
	query := func(api *testAPI, body string) resp {
		t.Helper()
		return call(t, api, http.MethodPost, "/api/v1/tenants:query", body, nil)
	}
	byStatus := `{"type": "probe.probe", "query": {"group": ["state"], "aggregate": {"n": {"count": "*"}}}}`

	// Every action of the built-in admin is not the right.
	if r := query(admin, byStatus); r.code != 403 {
		t.Fatalf("the built-in admin without the right: %d %s", r.code, r.raw)
	}
	// The right, named in a role at the root.
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "pl48_report", "label": {"de": "Auswertung über Mandanten"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/pl48_report/permissions", `{"type_name": "core.tenant", "action": "report_tenants", "scope": "all"}`, nil); r.code != 204 {
		t.Fatalf("permission: %d %s", r.code, r.raw)
	}
	// The administrator works in a tenant and gives his own person, which lives at the root,
	// the role there: the API checks the person of an account of the installation centrally
	// (before, with one database, 404).
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "pl48_report", "person_id": "`+admin.person.String()+`", "org_path": "root"}`, nil); r.code != 201 {
		t.Fatalf("assignment: %d %s", r.code, r.raw)
	}
	admin.authz.Invalidate(ctx)

	r := query(admin, byStatus)
	var got struct {
		Rows    []map[string]any `json:"rows"`
		Skipped []map[string]any `json:"skipped"`
	}
	if r.code != 200 || json.Unmarshal([]byte(r.raw), &got) != nil {
		t.Fatalf("by status: %d %s", r.code, r.raw)
	}
	var table []string
	for _, row := range got.Rows {
		table = append(table, fmt.Sprintf("%v %v %v", row["tenant"], row["state"], row["n"]))
	}
	sort.Strings(table)
	if fmt.Sprint(table) != "[root.ta new 2 root.tb new 1]" || len(got.Skipped) != 0 {
		t.Fatalf("by status: %v, skipped %v", table, got.Skipped)
	}

	// A list carries the tenant too; an unknown tenant is named; there is no paging.
	r = query(admin, `{"type": "probe.probe", "tenants": ["root.tb", "root.tc"], "query": {"fields": ["title"]}}`)
	if r.code != 200 || json.Unmarshal([]byte(r.raw), &got) != nil || len(got.Rows) != 1 || got.Rows[0]["tenant"] != "root.tb" ||
		len(got.Skipped) != 1 || got.Skipped[0]["tenant"] != "root.tc" || got.Skipped[0]["reason"] != "unknown" {
		t.Fatalf("list of B: %d %s", r.code, r.raw)
	}
	if r := query(admin, `{"type": "probe.probe", "query": {"after": "x"}}`); r.code != 422 {
		t.Fatalf("paging: %d %s", r.code, r.raw)
	}

	// An administrator of a tenant never has it.
	userA, _ := tenantUser(t, admin, "apitest_pl48", "root.ta")
	if r := query(userA, byStatus); r.code != 403 {
		t.Fatalf("an administrator of A: %d %s", r.code, r.raw)
	}

	// Each tenant has the evaluations in its security log, in its own database.
	for _, path := range []string{"root.ta", "root.tb"} {
		if n := count(t, admin.rt.For(path), "SELECT count(*) FROM security_event WHERE kind = 'report.tenants' AND tenant = $1::ltree", path); n == 0 {
			t.Errorf("no evaluation in the log of %s", path)
		}
	}
}
