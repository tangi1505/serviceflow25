// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Security settings over HTTP (PL28): the settings of the root node apply at once; a password
// too short for them is refused with the rule; the lockout and a switched-off local sign-in
// show in the security event log.
func TestSecuritySettingsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	set := func(key, value string) {
		t.Helper()
		if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/settings/"+key, value, nil); r.code != 204 {
			t.Fatalf("set %s: %d %s", key, r.code, r.raw)
		}
		// The test context is gone in the cleanup: remove the setting in the database directly,
		// so that no other test and no running instance keeps it.
		t.Cleanup(func() {
			if _, err := admin.pool.Exec(context.Background(), "DELETE FROM org_setting WHERE path = 'root' AND key = $1", key); err != nil {
				t.Errorf("remove %s: %v", key, err)
			}
		})
	}
	name := fmt.Sprintf("apitest_sec_%d", time.Now().UnixNano()%1000000)
	if _, err := admin.auth.CreateAccount(ctx, name, "Sec", "Wolke-Tisch-7!", nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.auth.Unlock(context.Background(), name) })

	// Minimum length 12: changing to a 9-character password is refused with the rule.
	set("security.password.min_length", "12")
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "Wolke-Tisch-7!"}`, map[string]string{"Cookie": ""})
	if login.code != 200 {
		t.Fatalf("login: %d %s", login.code, login.raw)
	}
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	r := call(t, user, http.MethodPost, "/api/v1/auth/password", `{"current_password": "Wolke-Tisch-7!", "new_password": "Kurz-Pw1!"}`, nil)
	if r.code != 422 || !strings.Contains(r.raw, "at least 12 characters") {
		t.Fatalf("short password: %d %s", r.code, r.raw)
	}
	if r := call(t, user, http.MethodPost, "/api/v1/auth/password", `{"current_password": "Wolke-Tisch-7!", "new_password": "Wolke-Tisch-Lampe-8!"}`, nil); r.code != 204 {
		t.Fatalf("long password: %d %s", r.code, r.raw)
	}

	// Lockout after three failures; the fourth attempt with the right password is refused.
	set("security.lockout.account_failures", "3")
	for range 3 {
		call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "falsch"}`, map[string]string{"Cookie": ""})
	}
	// Locked: 429 with the seconds left of the 15 minutes, no password checked (AP6).
	r = call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "Wolke-Tisch-Lampe-8!"}`, map[string]string{"Cookie": ""})
	if secs, _ := strconv.Atoi(r.hdr.Get("Retry-After")); r.code != 429 || secs < 1 || secs > 900 {
		t.Fatalf("locked: %d %s %v", r.code, r.raw, r.hdr)
	}
	if p := securityLog(t, admin, "/api/v1/audit/auth?limit=1&username="+name); len(p.Items) != 1 || p.Items[0].Reason != "locked" {
		t.Fatalf("lockout in the log: %+v", p.Items)
	}
	// Further refusals of the same lock within the minute are summed up, not written one by one
	// (AP6): still one entry with the reason locked.
	for range 3 {
		call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "falsch"}`, map[string]string{"Cookie": ""})
	}
	locked := 0
	for _, it := range securityLog(t, admin, "/api/v1/audit/auth?limit=20&username="+name).Items {
		if it.Reason == "locked" {
			locked++
		}
	}
	if locked != 1 {
		t.Fatalf("refusals of one lock in one minute: %d entries, want 1", locked)
	}
	if err := admin.auth.Unlock(ctx, name); err != nil {
		t.Fatal(err)
	}

	// Local sign-in switched off: the right password does not get in, the log says why; the
	// administrator (manage_rights) still gets in with "admins".
	set("security.local_login", `"admins"`)
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "Wolke-Tisch-Lampe-8!"}`, map[string]string{"Cookie": ""}); r.code != 401 {
		t.Fatalf("local login for a non-admin: %d %s", r.code, r.raw)
	}
	if p := securityLog(t, admin, "/api/v1/audit/auth?limit=1&username="+name); len(p.Items) != 1 || p.Items[0].Reason != "local_login_disabled" {
		t.Fatalf("local login in the log: %+v", p.Items)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""}); r.code != 200 {
		t.Fatalf("local login for the administrator: %d %s", r.code, r.raw)
	}
}
