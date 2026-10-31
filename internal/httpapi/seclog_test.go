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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type securityPage struct {
	Items []struct {
		ID        int64           `json:"id"`
		At        time.Time       `json:"at"`
		Kind      string          `json:"kind"`
		Outcome   string          `json:"outcome"`
		Account   *uuid.UUID      `json:"account"`
		Username  string          `json:"username"`
		IP        string          `json:"ip"`
		UserAgent string          `json:"user_agent"`
		Reason    string          `json:"reason"`
		Target    string          `json:"target"`
		Details   json.RawMessage `json:"details"`
	} `json:"items"`
	Next int64 `json:"next"`
}

func securityLog(t *testing.T, api *testAPI, path string) securityPage {
	t.Helper()
	r := call(t, api, http.MethodGet, path, "", nil)
	if r.code != 200 {
		t.Fatalf("%s: %d %s", path, r.code, r.raw)
	}
	var p securityPage
	if err := json.Unmarshal([]byte(r.raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// The acceptance of PL27: three failed sign-ins appear with time and source in
// /api/v1/audit/auth; a successful one and the sign-out too; changes of rights and settings
// appear with before and after in /api/v1/audit/changes; only who may read the log reads it.
func TestSecurityEventLog(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	name := fmt.Sprintf("apitest_audit_%d", time.Now().UnixNano()%1000000)
	acc, err := admin.auth.CreateAccount(ctx, name, "Audit", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	ua := map[string]string{"Cookie": "", "User-Agent": "pl27-acceptance"}
	since := time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	for _, pw := range []string{"wrong-1", "wrong-2"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "`+pw+`"}`, ua); r.code != 401 {
			t.Fatalf("wrong password: %d", r.code)
		}
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`_nobody", "password": "x"}`, ua); r.code != 401 {
		t.Fatalf("unknown account: %d", r.code)
	}

	failed := securityLog(t, admin, "/api/v1/audit/auth?outcome=failure&since="+since+"&username="+name)
	if len(failed.Items) != 2 {
		t.Fatalf("failed sign-ins of %s: %+v", name, failed.Items)
	}
	for _, e := range failed.Items {
		if e.Kind != "login" || e.Reason != "wrong_password" || e.Account == nil || *e.Account != acc.ID || e.IP != "127.0.0.1" ||
			e.UserAgent != "pl27-acceptance" || time.Since(e.At) > time.Minute {
			t.Fatalf("failed sign-in: %+v", e)
		}
	}
	unknown := securityLog(t, admin, "/api/v1/audit/auth?username="+name+"_nobody")
	if len(unknown.Items) != 1 || unknown.Items[0].Reason != "unknown_account" || unknown.Items[0].Account != nil {
		t.Fatalf("unknown account: %+v", unknown.Items)
	}
	// Paging by key, newest first.
	first := securityLog(t, admin, "/api/v1/audit/auth?limit=1&username="+name)
	second := securityLog(t, admin, fmt.Sprintf("/api/v1/audit/auth?limit=1&username=%s&before=%d", name, first.Next))
	if len(first.Items) != 1 || first.Next == 0 || len(second.Items) != 1 || second.Items[0].ID >= first.Items[0].ID {
		t.Fatalf("paging: %+v / %+v", first, second)
	}

	// A successful sign-in and the sign-out.
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "`+testPassword+`"}`, ua)
	if login.code != 200 {
		t.Fatalf("login: %d %s", login.code, login.raw)
	}
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	if r := call(t, user, http.MethodGet, "/api/v1/audit/auth", "", nil); r.code != 403 {
		t.Fatalf("reading the log without the right: %d %s", r.code, r.raw)
	}
	if r := call(t, user, http.MethodPost, "/api/v1/auth/logout", "", nil); r.code != 204 {
		t.Fatalf("logout: %d %s", r.code, r.raw)
	}
	ok := securityLog(t, admin, "/api/v1/audit/auth?outcome=success&username="+name)
	if len(ok.Items) != 2 || ok.Items[0].Kind != "logout" || ok.Items[1].Kind != "login" || ok.Items[1].Account == nil {
		t.Fatalf("success: %+v", ok.Items)
	}

	// Changes of rights and settings, with before and after.
	role := strings.ReplaceAll(name, "apitest_audit_", "audit_role_")
	dropRole(t, admin, role)
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`"}`, nil); r.code != 201 {
		t.Fatalf("create role: %d %s", r.code, r.raw)
	}
	// The same scope twice replaces the line (with a condition the second time); since UI4 a
	// role may hold several scopes, so the change of a line names the line of its own scope.
	for _, body := range []string{`"scope": "own"`, `"scope": "own", "condition": "record.priority > 1"`} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/"+role+"/permissions", `{"type_name": "probe.probe", "action": "read", `+body+`}`, nil); r.code != 204 {
			t.Fatalf("set permission: %d %s", r.code, r.raw)
		}
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/roles/"+role, "", nil); r.code != 204 {
		t.Fatalf("delete role: %d %s", r.code, r.raw)
	}
	changes := securityLog(t, admin, "/api/v1/audit/changes?since="+since+"&kind=permission.set")
	var mine []string
	for _, e := range changes.Items {
		if strings.HasPrefix(e.Target, role+" ") {
			mine = append(mine, string(e.Details))
		}
	}
	// jsonb orders the keys its own way: scope comes first.
	if len(mine) != 2 || !strings.Contains(mine[0], `"before":{"scope":"own"`) ||
		!strings.Contains(mine[0], `"condition":"record.priority`) || strings.Contains(mine[1], `"before"`) {
		t.Fatalf("permission changes: %v", mine)
	}
	deleted := securityLog(t, admin, "/api/v1/audit/changes?kind=role.delete&since="+since)
	found := false
	for _, e := range deleted.Items {
		found = found || (e.Target == role && strings.Contains(string(e.Details), `"before"`) && e.Username == "apitest")
	}
	if !found {
		t.Fatalf("role.delete: %+v", deleted.Items)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/settings/audit.security_retention", `400`, nil); r.code != 204 {
		t.Fatalf("set setting: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root/settings/audit.security_retention", "", nil); r.code != 204 {
		t.Fatalf("unset setting: %d %s", r.code, r.raw)
	}
	settings := securityLog(t, admin, "/api/v1/audit/changes?kind=setting.unset&since="+since)
	if len(settings.Items) == 0 || settings.Items[0].Target != "root audit.security_retention" || !strings.Contains(string(settings.Items[0].Details), `"before":400`) {
		t.Fatalf("setting.unset: %+v", settings.Items)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/audit/auth?ip=nonsense", "", nil); r.code != 400 {
		t.Fatalf("bad ip: %d", r.code)
	}
}
