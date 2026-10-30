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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/records"
)

// Webhooks over HTTP (PL25): create with the secret once, read without it, change, rotate,
// remove; https only; a record's event becomes a queued delivery for the admin's subscription;
// without the right 403.
func TestWebhooksOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	created := call(t, admin, http.MethodPost, "/api/v1/webhooks",
		`{"name": "Monitoring", "url": "https://receiver.example/hook", "types": ["probe.probe"], "kinds": ["create"]}`, nil)
	if created.code != 201 || !strings.HasPrefix(created.body["secret"].(string), "whsec_") || created.body["enabled"] != true {
		t.Fatalf("create: %d %s", created.code, created.raw)
	}
	base := "/api/v1/webhooks/" + created.body["id"].(string)
	if r := call(t, admin, http.MethodGet, base, "", nil); r.code != 200 || strings.Contains(r.raw, "whsec_") {
		t.Fatalf("get shows no secret: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/webhooks", "", nil); r.code != 200 || strings.Contains(r.raw, "whsec_") || !strings.Contains(r.raw, "Monitoring") {
		t.Fatalf("list: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/webhooks", `{"name": "plain", "url": "http://receiver.example/hook"}`, nil); r.code != 422 || !strings.Contains(r.raw, "https") {
		t.Fatalf("http target: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPatch, base, `{"name": "Monitoring 2"}`, nil); r.code != 200 || r.body["name"] != "Monitoring 2" {
		t.Fatalf("update: %d %s", r.code, r.raw)
	}
	rotated := call(t, admin, http.MethodPost, base+":rotate", "", nil)
	if rotated.code != 200 || rotated.body["secret"] == created.body["secret"] {
		t.Fatalf("rotate: %d %s", rotated.code, rotated.raw)
	}

	// A new probe record: the dispatcher hands its event to the webhooks, a delivery is queued.
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "webhook"}`, nil)
	id := uuid.MustParse(rec.body["id"].(string))
	var e records.Event
	for deadline := time.Now().Add(10 * time.Second); e.Position == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		list, err := records.New(admin.pool).Events(ctx, int64(created.body["position"].(float64)), 1000)
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range list {
			if ev.RecordID == id {
				e = ev
			}
		}
	}
	if err := admin.hooks.Handle(ctx, e); err != nil {
		t.Fatal(err)
	}
	// One delivery with 8 attempts. Its state depends on who took it: a running instance on the
	// same database has its own dispatcher and workers (PL41) and may already be at it.
	r := call(t, admin, http.MethodGet, base+"/deliveries", "", nil)
	var deliveries []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &deliveries)
	if r.code != 200 || len(deliveries) != 1 || deliveries[0]["max_attempts"] != float64(8) || deliveries[0]["position"] != float64(e.Position) {
		t.Fatalf("deliveries: %d %s", r.code, r.raw)
	}

	// Without the right: 403.
	acc, err := admin.auth.CreateAccount(ctx, "apitest_hooks", "Hooks", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "Hooks"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_hooks", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	other := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	if r := call(t, other, http.MethodGet, "/api/v1/webhooks", "", nil); r.code != 403 {
		t.Fatalf("list without the right: %d %s", r.code, r.raw)
	}

	if r := call(t, admin, http.MethodDelete, base, "", nil); r.code != 204 {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base, "", nil); r.code != 404 {
		t.Fatalf("get deleted: %d %s", r.code, r.raw)
	}
}
