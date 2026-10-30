// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Search over HTTP (PL26): hits with type, title and snippet; the rights of the session
// decide (a person who may read nothing finds nothing); unknown types are 404.
func TestSearchOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	word := fmt.Sprintf("Plotter%d", time.Now().UnixNano()%1000000)
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "`+word+` im Keller defekt"}`, nil)
	if rec.code != 201 {
		t.Fatalf("create: %d %s", rec.code, rec.raw)
	}
	r := call(t, admin, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(word), "", nil)
	if r.code != 200 || !strings.Contains(r.raw, `"type":"probe.probe"`) || !strings.Contains(r.raw, rec.body["id"].(string)) ||
		!strings.Contains(r.raw, "u003cmark") {
		t.Fatalf("search: %d %s", r.code, r.raw)
	}
	// Narrowed to a type without the record: nothing; an unknown type: 404.
	if r := call(t, admin, http.MethodGet, "/api/v1/search?types=core.person&q="+url.QueryEscape(word), "", nil); r.code != 200 || strings.TrimSpace(r.raw) != "[]" {
		t.Fatalf("narrowed: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/search?types=no.such&q=x", "", nil); r.code != 404 {
		t.Fatalf("unknown type: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/search", "", nil); r.code != 400 {
		t.Fatalf("without q: %d %s", r.code, r.raw)
	}

	// A person without rights on probe.probe finds nothing.
	acc, err := admin.auth.CreateAccount(ctx, "apitest_search", "Search", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "Search"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_search", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	other := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	if r := call(t, other, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(word), "", nil); r.code != 200 || strings.Contains(r.raw, rec.body["id"].(string)) {
		t.Fatalf("search without rights: %d %s", r.code, r.raw)
	}
}
