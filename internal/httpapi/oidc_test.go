// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"group24.de/itsm/internal/authn/authntest"
)

// The Prüfweg of PL11 against the fake provider: start, provider, callback, session; the
// first sign-in creates account and person; a login rule assigns a role from the groups and
// takes it away when the group is gone; a disabled account and a stale state are refused.
func TestSignInThroughProvider(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	anon := &testAPI{Server: admin.Server}
	// Earlier runs leave the accounts (and their identities) behind; the provider's address
	// differs per run, so they would get a suffix otherwise.
	for _, sql := range []string{
		"DELETE FROM account WHERE username LIKE 'max_muster%'",
		"DELETE FROM rec_core_person WHERE email = 'max@example.org' OR last_name = 'Other Max'",
	} {
		if _, err := admin.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}

	r := call(t, anon, http.MethodGet, "/api/v1/auth/providers", "", nil)
	var providers []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &providers)
	if r.code != 200 || len(providers) != 1 || providers[0]["name"] != "Fake IdP" || providers[0]["start_url"] != "/api/v1/auth/oidc/start" {
		t.Fatalf("providers: %d %s", r.code, r.raw)
	}

	// A rule: members of itsm-readers read every probe record.
	dropRole(t, admin, "sso_reader")
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "sso_reader"}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/sso_reader/permissions", `{"type_name": "probe.probe", "action": "read", "scope": "all"}`, nil); r.code != 204 {
		t.Fatalf("permission: %d %s", r.code, r.raw)
	}
	rules := call(t, admin, http.MethodGet, "/api/v1/auth/rules", "", nil)
	var existing []map[string]any
	_ = json.Unmarshal([]byte(rules.raw), &existing)
	for _, e := range existing {
		call(t, admin, http.MethodDelete, "/api/v1/auth/rules/"+e["id"].(string), "", nil)
	}
	rule := call(t, admin, http.MethodPost, "/api/v1/auth/rules", `{"name": "sso-readers", "condition": "\"itsm-readers\" in claims.groups", "role": "sso_reader"}`, nil)
	if rule.code != 201 || rule.body["org_path"] != "root" || rule.body["enabled"] != true || rule.body["position"] != 100.0 {
		t.Fatalf("rule: %d %s", rule.code, rule.raw)
	}
	ruleID := rule.body["id"].(string)
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/rules", `{"name": "bad", "condition": "claims.groups +", "role": "sso_reader"}`, nil); r.code != 422 {
		t.Fatalf("bad condition: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/rules", `{"name": "bad", "condition": "true", "role": "no_such_role"}`, nil); r.code != 422 {
		t.Fatalf("unknown role: %d %s", r.code, r.raw)
	}
	if r := call(t, anon, http.MethodGet, "/api/v1/auth/rules", "", map[string]string{"Cookie": ""}); r.code != 401 {
		t.Fatalf("rules without session: %d", r.code)
	}

	// The browser: follows no redirects itself, so every hop is visible.
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(target string) *http.Response { //nolint:bodyclose // every body is closed right here, before the response is returned
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := browser.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	signIn := func(next string) *http.Response { //nolint:bodyclose // see get
		t.Helper()
		start := get(admin.URL + "/api/v1/auth/oidc/start?next=" + url.QueryEscape(next))
		loc := start.Header.Get("Location")
		if start.StatusCode != 302 || !strings.HasPrefix(loc, admin.idp.Issuer+"/auth?") {
			t.Fatalf("start: %d %s", start.StatusCode, loc)
		}
		q, _ := url.Parse(loc)
		if v := q.Query(); v.Get("state") == "" || v.Get("nonce") == "" || v.Get("code_challenge_method") != "S256" || v.Get("scope") != "openid profile email" {
			t.Fatalf("authorization request: %s", loc)
		}
		back := get(loc)
		if back.StatusCode != 302 || !strings.HasPrefix(back.Header.Get("Location"), admin.URL+"/api/v1/auth/oidc/callback?") {
			t.Fatalf("provider: %d %s", back.StatusCode, back.Header.Get("Location"))
		}
		return get(back.Header.Get("Location"))
	}

	admin.idp.Login(authntest.User{Subject: "sub-1", Username: "Max.Muster", Name: "Max Muster", GivenName: "Max", FamilyName: "Muster", Email: "max@example.org", Groups: []string{"itsm-readers"}})
	done := signIn("/types/probe.probe") //nolint:bodyclose // closed in get
	if done.StatusCode != 302 || done.Header.Get("Location") != "/types/probe.probe" {
		t.Fatalf("callback: %d %s", done.StatusCode, done.Header.Get("Location"))
	}
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, done.Header)}
	me := call(t, user, http.MethodGet, "/api/v1/auth/me", "", nil)
	if me.code != 200 || me.body["username"] != "max_muster" || me.body["display_name"] != "Max Muster" {
		t.Fatalf("me: %d %s", me.code, me.raw)
	}
	roles, _ := me.body["roles"].([]any)
	if len(roles) != 1 || roles[0] != "sso_reader" {
		t.Fatalf("role from the rule: %s", me.raw)
	}
	if r := call(t, user, http.MethodGet, "/api/v1/records/probe.probe", "", nil); r.code != 200 {
		t.Fatalf("reading with the role: %d %s", r.code, r.raw)
	}
	// The person carries name and email; the account has no local password.
	persons := call(t, admin, http.MethodGet, "/api/v1/records/core.person?filter[email]=max@example.org", "", nil)
	var page struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal([]byte(persons.raw), &page)
	if persons.code != 200 || len(page.Items) != 1 || page.Items[0]["last_name"] != "Muster" || page.Items[0]["first_name"] != "Max" {
		t.Fatalf("person: %d %s", persons.code, persons.raw)
	}
	if r := call(t, anon, http.MethodPost, "/api/v1/auth/login", `{"username": "max_muster", "password": "anything"}`, map[string]string{"Cookie": ""}); r.code != 401 {
		t.Fatalf("local sign-in of a provider account: %d %s", r.code, r.raw)
	}
	assignments := call(t, admin, http.MethodGet, "/api/v1/assignments", "", nil)
	if !strings.Contains(assignments.raw, `"sso_reader"`) {
		t.Fatalf("assignment of the rule missing: %s", assignments.raw)
	}

	// Second sign-in without the group: same account, role gone, no second person.
	admin.idp.Login(authntest.User{Subject: "sub-1", Username: "Max.Muster", Name: "Max Muster", Email: "max@example.org"})
	again := signIn("/") //nolint:bodyclose // closed in get
	user2 := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, again.Header)}
	me = call(t, user2, http.MethodGet, "/api/v1/auth/me", "", nil)
	roles, _ = me.body["roles"].([]any)
	if me.code != 200 || me.body["username"] != "max_muster" || len(roles) != 0 {
		t.Fatalf("after the group is gone: %d %s", me.code, me.raw)
	}
	persons = call(t, admin, http.MethodGet, "/api/v1/records/core.person?filter[email]=max@example.org", "", nil)
	_ = json.Unmarshal([]byte(persons.raw), &page)
	if len(page.Items) != 1 {
		t.Fatalf("one person, not %d", len(page.Items))
	}

	// A username that is taken gets a suffix.
	admin.idp.Login(authntest.User{Subject: "sub-2", Username: "max.muster", Name: "Other Max", Groups: []string{"itsm-readers"}})
	third := signIn("/") //nolint:bodyclose // closed in get
	me = call(t, &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, third.Header)}, http.MethodGet, "/api/v1/auth/me", "", nil)
	if me.body["username"] != "max_muster_2" {
		t.Fatalf("second subject with the same name: %s", me.raw)
	}

	// Refusals: a disabled account, a stale state, a provider error.
	if err := admin.auth.SetDisabled(ctx, "max_muster", true); err != nil {
		t.Fatal(err)
	}
	admin.idp.Login(authntest.User{Subject: "sub-1", Username: "Max.Muster", Name: "Max Muster"})
	if res := signIn("/"); res.Header.Get("Location") != "/login?error=disabled" {
		t.Fatalf("disabled: %s", res.Header.Get("Location"))
	}
	if res := get(admin.URL + "/api/v1/auth/oidc/callback?state=nope&code=x"); res.Header.Get("Location") != "/login?error=state" {
		t.Fatalf("stale state: %s", res.Header.Get("Location"))
	}
	if res := get(admin.URL + "/api/v1/auth/oidc/callback?error=access_denied"); res.Header.Get("Location") != "/login?error=refused" {
		t.Fatalf("provider error: %s", res.Header.Get("Location"))
	}

	// Rules: update and delete; deleting takes the assignments with it.
	if r := call(t, admin, http.MethodPut, "/api/v1/auth/rules/"+ruleID, `{"name": "sso-readers", "condition": "\"itsm-readers\" in claims.groups", "role": "sso_reader", "enabled": false, "position": 5}`, nil); r.code != 200 || r.body["enabled"] != false || r.body["position"] != 5.0 {
		t.Fatalf("update rule: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/auth/rules/"+ruleID, "", nil); r.code != 204 {
		t.Fatalf("delete rule: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/auth/rules/"+ruleID, "", nil); r.code != 404 {
		t.Fatalf("delete twice: %d %s", r.code, r.raw)
	}
	assignments = call(t, admin, http.MethodGet, "/api/v1/assignments", "", nil)
	if strings.Contains(assignments.raw, `"sso_reader"`) {
		t.Fatalf("assignments of the rule must be gone: %s", assignments.raw)
	}
}
