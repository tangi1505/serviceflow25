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
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The Prüfweg of PL13: a client gets a token with id and secret, calls the API as its
// account with the roles of its person; a wrong secret is 401, an address outside the
// ranges 403, the request over the rate 429 with Retry-After; disabling, rotating and
// deleting the client end its tokens.
func TestAPIClients(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	for _, sql := range []string{
		"DELETE FROM api_client WHERE client_id LIKE 'apitest%'",
		"DELETE FROM account WHERE username LIKE 'apitest_client%'",
		"DELETE FROM rec_core_person WHERE last_name LIKE 'apitest client%'",
		"DELETE FROM org_setting WHERE key LIKE 'api.%'",
	} {
		if _, err := admin.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	// The rate counters of an earlier run in the same minute would count against this one.
	minute := time.Now().Unix() / 60
	for _, key := range []string{fmt.Sprintf("rate:apitest_client:%d", minute), fmt.Sprintf("rate:apitest_client:%d", minute-1), "token:of:apitest_client"} {
		if err := admin.kv.Del(ctx, key); err != nil {
			t.Fatal(err)
		}
	}

	r := call(t, admin, http.MethodPost, "/api/v1/clients", `{"client_id": "apitest_client", "name": "apitest client", "rate_limit": 3}`, nil)
	if r.code != 201 || r.body["secret"] == nil || r.body["person_id"] == nil || r.body["enabled"] != true {
		t.Fatalf("create client: %d %s", r.code, r.raw)
	}
	secret := r.body["secret"].(string)
	personID := r.body["person_id"].(string)
	if r := call(t, admin, http.MethodPost, "/api/v1/clients", `{"client_id": "apitest_client"}`, nil); r.code != 422 {
		t.Fatalf("duplicate client: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/clients", `{"client_id": "apitest_client2", "ip_ranges": ["nonsense"]}`, nil); r.code != 422 {
		t.Fatalf("bad range: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/clients/apitest_client", "", nil); r.code != 200 || r.body["secret"] != nil || r.body["secret_hash"] != nil {
		t.Fatalf("get client must not show the secret: %d %s", r.code, r.raw)
	}
	// The client's person gets a role: read every probe record.
	dropRole(t, admin, "apitest_reader")
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "apitest_reader"}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/apitest_reader/permissions", `{"type_name": "probe.probe", "action": "read", "scope": "all"}`, nil); r.code != 204 {
		t.Fatalf("permission: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "apitest_reader", "person_id": "`+personID+`", "org_path": "root"}`, nil); r.code != 201 {
		t.Fatalf("assign: %d %s", r.code, r.raw)
	}

	// The token endpoint: a form without CSRF header and without cookie, like curl.
	token := func(id, secret string) (int, map[string]any) {
		t.Helper()
		form := url.Values{"grant_type": {"client_credentials"}, "client_id": {id}, "client_secret": {secret}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, admin.URL+"/api/v1/auth/token", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		return res.StatusCode, body
	}
	code, body := token("apitest_client", "wrong")
	if code != 401 || body["error"] != "invalid_client" {
		t.Fatalf("wrong secret: %d %v", code, body)
	}
	code, body = token("apitest_client", secret)
	if code != 200 || body["token_type"] != "Bearer" || body["expires_in"] != 3600.0 || body["access_token"] == nil {
		t.Fatalf("token: %d %v", code, body)
	}
	access := body["access_token"].(string)

	// Calls with the token: as the client's account, with its role; the fourth within a
	// minute is over the limit of three.
	bearer := func(path string) resp {
		t.Helper()
		return call(t, &testAPI{Server: admin.Server}, http.MethodGet, path, "", map[string]string{"Cookie": "", "Authorization": "Bearer " + access})
	}
	me := bearer("/api/v1/auth/me")
	if me.code != 200 || me.body["username"] != "apitest_client" || len(me.body["roles"].([]any)) != 1 {
		t.Fatalf("me with token: %d %s", me.code, me.raw)
	}
	if r := bearer("/api/v1/records/probe.probe"); r.code != 200 {
		t.Fatalf("list with token: %d %s", r.code, r.raw)
	}
	if r := bearer("/api/v1/records/core.person"); r.code != 200 || !strings.Contains(r.raw, `"items":[]`) {
		t.Fatalf("no role on persons shows nothing: %d %s", r.code, r.raw)
	}
	limited := bearer("/api/v1/auth/me")
	if limited.code != 429 || limited.hdr.Get("Retry-After") == "" || limited.body["type"] != "/problems/rate_limited" {
		t.Fatalf("fourth call in the minute: %d %s %s", limited.code, limited.hdr.Get("Retry-After"), limited.raw)
	}
	if r := call(t, &testAPI{Server: admin.Server}, http.MethodGet, "/api/v1/auth/me", "", map[string]string{"Cookie": "", "Authorization": "Bearer nope"}); r.code != 401 {
		t.Fatalf("unknown token: %d %s", r.code, r.raw)
	}

	// Addresses: the test calls come from 127.0.0.1; a client limited to 10.0.0.0/8 is refused
	// at the token endpoint and with an existing token.
	if r := call(t, admin, http.MethodPut, "/api/v1/clients/apitest_client", `{"ip_ranges": ["10.0.0.0/8"], "rate_limit": 1000}`, nil); r.code != 200 || r.body["ip_ranges"].([]any)[0] != "10.0.0.0/8" {
		t.Fatalf("update ranges: %d %s", r.code, r.raw)
	}
	if code, body := token("apitest_client", secret); code != 403 || body["error"] != "access_denied" {
		t.Fatalf("token from outside the range: %d %v", code, body)
	}
	if r := bearer("/api/v1/auth/me"); r.code != 403 {
		t.Fatalf("call from outside the range: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/clients/apitest_client", `{"ip_ranges": ["127.0.0.1", "::1"]}`, nil); r.code != 200 {
		t.Fatalf("update ranges back: %d %s", r.code, r.raw)
	}
	if r := bearer("/api/v1/auth/me"); r.code != 200 {
		t.Fatalf("call from inside the range: %d %s", r.code, r.raw)
	}

	// Settings of the root node give the defaults: a client without its own limit.
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/settings/api.token_ttl", `120`, nil); r.code != 204 && r.code != 200 {
		t.Fatalf("setting: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/clients", `{"client_id": "apitest_client2", "name": "apitest client 2"}`, nil); r.code != 201 {
		t.Fatalf("second client: %d %s", r.code, r.raw)
	} else if code, body := token("apitest_client2", r.body["secret"].(string)); code != 200 || body["expires_in"] != 120.0 {
		t.Fatalf("token ttl from the setting: %d %v", code, body)
	}

	// Disabling ends the tokens; rotating ends the old secret; deleting ends everything.
	if r := call(t, admin, http.MethodPut, "/api/v1/clients/apitest_client", `{"enabled": false}`, nil); r.code != 200 {
		t.Fatalf("disable: %d %s", r.code, r.raw)
	}
	if r := bearer("/api/v1/auth/me"); r.code != 401 {
		t.Fatalf("token of a disabled client: %d %s", r.code, r.raw)
	}
	if code, _ := token("apitest_client", secret); code != 401 {
		t.Fatalf("token for a disabled client: %d", code)
	}
	call(t, admin, http.MethodPut, "/api/v1/clients/apitest_client", `{"enabled": true}`, nil)
	rot := call(t, admin, http.MethodPost, "/api/v1/clients/apitest_client:rotate", "", nil)
	if rot.code != 200 || rot.body["secret"] == nil || rot.body["secret"] == secret {
		t.Fatalf("rotate: %d %s", rot.code, rot.raw)
	}
	if code, _ := token("apitest_client", secret); code != 401 {
		t.Fatalf("old secret after rotation: %d", code)
	}
	if code, _ := token("apitest_client", rot.body["secret"].(string)); code != 200 {
		t.Fatalf("new secret: %d", code)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/clients/apitest_client", "", nil); r.code != 204 {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	if code, _ := token("apitest_client", rot.body["secret"].(string)); code != 401 {
		t.Fatalf("token after delete: %d", code)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/clients/apitest_client", "", nil); r.code != 404 {
		t.Fatalf("deleted client: %d", r.code)
	}
	// Without manage_rights nothing of this is reachable.
	if _, err := admin.auth.CreateAccount(ctx, "apitest_client_nobody", "nobody", testPassword, nil); err != nil {
		t.Fatal(err)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_client_nobody", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	nobody := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	if r := call(t, nobody, http.MethodGet, "/api/v1/clients", "", nil); r.code != 403 {
		t.Fatalf("clients without the right: %d", r.code)
	}
}
