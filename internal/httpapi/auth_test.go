// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// The Prüfweg of PL5 over HTTP: with cookie 200, without 401, after logout 401; plus the CSRF
// header (E74), the expired password (E77) and the public endpoints (E78).
func TestSessionGuardsTheAPI(t *testing.T) {
	api := apiServer(t)
	const base = "/api/v1/records/probe.probe"

	// Without the session only version and login are open.
	anon := map[string]string{"Cookie": ""}
	if r := call(t, api, http.MethodGet, base, "", anon); r.code != 401 || r.body["type"] != "/problems/unauthorized" {
		t.Fatalf("without cookie: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/types", "", anon); r.code != 401 {
		t.Fatalf("types without cookie: %d", r.code)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/openapi.json", "", anon); r.code != 401 {
		t.Fatalf("openapi without cookie: %d", r.code)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/version", "", anon); r.code != 200 {
		t.Fatalf("version without cookie: %d", r.code)
	}
	if r := call(t, api, http.MethodGet, "/healthz", "", anon); r.code != 200 {
		t.Fatalf("healthz without cookie: %d", r.code)
	}
	if r := call(t, api, http.MethodGet, base, "", map[string]string{"Cookie": sessionCookie + "=nonsense"}); r.code != 401 {
		t.Fatalf("bogus cookie: %d", r.code)
	}

	// With the session the API answers, and the session appears as created_by.
	r := call(t, api, http.MethodPost, base, `{"title": "mine"}`, nil)
	if r.code != 201 {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	if r.body["created_by"] == nil || r.body["created_by"] == "" {
		t.Fatalf("created_by must be the account: %s", r.raw)
	}
	me := call(t, api, http.MethodGet, "/api/v1/auth/me", "", nil)
	if me.code != 200 || me.body["username"] != "apitest" || me.body["id"] != r.body["created_by"] {
		t.Fatalf("me: %d %s (created_by %v)", me.code, me.raw, r.body["created_by"])
	}
	if exp, ok := me.body["session_expires_at"].(string); !ok || exp == "" {
		t.Fatalf("session_expires_at missing: %s", me.raw)
	}

	// Writes need the CSRF header; reads do not.
	if r := call(t, api, http.MethodPost, base, `{"title": "csrf"}`, map[string]string{csrfHeader: ""}); r.code != 403 || r.body["type"] != "/problems/forbidden" {
		t.Fatalf("without CSRF header: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodGet, base, "", map[string]string{csrfHeader: ""}); r.code != 200 {
		t.Fatalf("read without CSRF header: %d", r.code)
	}
	if r := call(t, api, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest", "password": "`+testPassword+`"}`, map[string]string{csrfHeader: "", "Cookie": ""}); r.code != 403 {
		t.Fatalf("login without CSRF header: %d", r.code)
	}

	// Wrong credentials are 401 without telling why; a bad body is 422.
	if r := call(t, api, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest", "password": "nope"}`, anon); r.code != 401 || r.body["type"] != "/problems/invalid_credentials" {
		t.Fatalf("wrong password: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest"}`, anon); r.code != 422 {
		t.Fatalf("missing password: %d %s", r.code, r.raw)
	}

	// The cookie is HttpOnly and SameSite=Lax; without TLS it is not Secure.
	login := call(t, api, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest", "password": "`+testPassword+`"}`, anon)
	if login.code != 200 {
		t.Fatalf("login: %d %s", login.code, login.raw)
	}
	sc := strings.Join(login.hdr.Values("Set-Cookie"), ";")
	for _, want := range []string{"HttpOnly", "SameSite=Lax", "Path=/"} {
		if !strings.Contains(sc, want) {
			t.Fatalf("Set-Cookie lacks %s: %s", want, sc)
		}
	}
	if strings.Contains(sc, "Secure") {
		t.Fatalf("Secure without TLS: %s", sc)
	}
	second := &testAPI{Server: api.Server, cookie: sessionCookieOf(t, login.hdr)}

	// Logout ends that session and no other.
	if r := call(t, second, http.MethodPost, "/api/v1/auth/logout", "", nil); r.code != 204 {
		t.Fatalf("logout: %d %s", r.code, r.raw)
	}
	if r := call(t, second, http.MethodGet, base, "", nil); r.code != 401 {
		t.Fatalf("after logout: %d", r.code)
	}
	if r := call(t, api, http.MethodGet, base, "", nil); r.code != 200 {
		t.Fatalf("other session after logout: %d", r.code)
	}
}

func TestExpiredPasswordAllowsOnlyTheChange(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	past := time.Now().Add(-time.Hour)
	if _, err := api.auth.CreateAccount(ctx, "apitest_expired", "Expired", "Old-Pass1!", &past); err != nil {
		t.Fatal(err)
	}
	login := call(t, api, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_expired", "password": "Old-Pass1!"}`, map[string]string{"Cookie": ""})
	if login.code != 200 || login.body["password_expired"] != true {
		t.Fatalf("login with expired password: %d %s", login.code, login.raw)
	}
	exp := &testAPI{Server: api.Server, cookie: sessionCookieOf(t, login.hdr)}
	if r := call(t, exp, http.MethodGet, "/api/v1/records/probe.probe", "", nil); r.code != 403 || r.body["type"] != "/problems/password_expired" {
		t.Fatalf("records while expired: %d %s", r.code, r.raw)
	}
	if r := call(t, exp, http.MethodGet, "/api/v1/auth/me", "", nil); r.code != 200 {
		t.Fatalf("me while expired: %d", r.code)
	}
	if r := call(t, exp, http.MethodPost, "/api/v1/auth/password", `{"current_password": "Old-Pass1!", "new_password": "weak"}`, nil); r.code != 422 {
		t.Fatalf("weak new password: %d %s", r.code, r.raw)
	}
	if r := call(t, exp, http.MethodPost, "/api/v1/auth/password", `{"current_password": "wrong", "new_password": "New-Pass1!"}`, nil); r.code != 422 {
		t.Fatalf("wrong current password: %d %s", r.code, r.raw)
	}
	if r := call(t, exp, http.MethodPost, "/api/v1/auth/password", `{"current_password": "Old-Pass1!", "new_password": "New-Pass1!"}`, nil); r.code != 204 {
		t.Fatalf("change: %d %s", r.code, r.raw)
	}
	if r := call(t, exp, http.MethodGet, "/api/v1/records/probe.probe", "", nil); r.code != 200 {
		t.Fatalf("records after change: %d %s", r.code, r.raw)
	}
	if r := call(t, exp, http.MethodGet, "/api/v1/auth/me", "", nil); r.body["password_expired"] != false {
		t.Fatalf("me after change: %s", r.raw)
	}
}
