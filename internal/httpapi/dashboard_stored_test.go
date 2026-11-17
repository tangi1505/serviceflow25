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
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db/dbtest"
)

// memStore is a store of widgets in memory, so that tests do not share Valkey (PL68).
type memStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (s *memStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return nil, cache.ErrMiss
	}
	return v, nil
}

func (s *memStore) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}

func (s *memStore) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

// The Prüfweg of PL68 (Hauptplan E195): a long period shows the stored state with the moment
// it was computed; "fresh" computes again; 7 days always count fresh; a person with other
// rights has her own; 30 days moving with the clock find the same; the stream of a dashboard
// shows the stored state too.
func TestDashboardStoredStateOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_pl68"))
	api := apiServer(t)
	mem := &memStore{m: map[string][]byte{}}
	api.dashboards.Keep(mem)
	create := func(a *testAPI, body string) {
		t.Helper()
		if r := call(t, a, http.MethodPost, "/api/v1/records/probe.probe", body, nil); r.code != 201 {
			t.Fatalf("create: %d %s", r.code, r.raw)
		}
	}
	for _, title := range []string{"PL68 eins", "PL68 zwei", "PL68 drei"} {
		create(api, `{"title": "`+title+`", "kind": "fault"}`)
	}
	const role = "pl68_viewer"
	dropRole(t, api, role)
	call(t, api, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "Betrachter"}}`, nil)
	for _, p := range []string{`{"type_name": "probe.probe", "action": "read", "scope": "own"}`, `{"type_name": "probe.probe", "action": "create", "scope": "all"}`,
		`{"type_name": "core.dashboard", "action": "read", "scope": "involved"}`} {
		if r := call(t, api, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	viewer := timelineLogin(t, api, "apitest_pl68_viewer", role, `{"last_name": "Betrachterin"}`)
	d := call(t, api, http.MethodPost, "/api/v1/records/core.dashboard", `{"name": "PL68", "actors": {"shared": [{"type": "core.person", "id": "`+viewer.person+`"}]}}`, nil).body
	path := "/api/v1/dashboards/" + d["id"].(string)
	r := call(t, api, http.MethodPut, path, fmt.Sprintf(`{"version": %v, "widgets": [{"title": "Alle", "kind": "number", "target_type": "probe.probe", "x": 0, "y": 0}]}`, d["version"]), nil)
	if r.code != 200 {
		t.Fatalf("save: %d %s", r.code, r.raw)
	}
	widget := r.body["widgets"].([]any)[0].(map[string]any)["id"].(string)
	data := func(a *testAPI, filters string) (float64, string) {
		t.Helper()
		r := call(t, a, http.MethodPost, path+"/widgets/"+widget+"/data", filters, nil)
		if r.code != 200 {
			t.Fatalf("data %s: %d %s", filters, r.code, r.raw)
		}
		at, _ := r.body["computed_at"].(string)
		if at == "" {
			t.Fatalf("no moment: %s", r.raw)
		}
		return r.body["value"].(float64), at
	}

	// Over everything: computed once, then the stored state until "fresh".
	v, first := data(api, `{}`)
	if v != 3 {
		t.Fatalf("first: %v", v)
	}
	create(api, `{"title": "PL68 vier", "kind": "fault"}`)
	if v, at := data(api, `{}`); v != 3 || at != first {
		t.Fatalf("stored: %v %s, first %s", v, at, first)
	}
	v, again := data(api, `{"fresh": true}`)
	if v != 4 || again == first {
		t.Fatalf("fresh: %v %s", v, again)
	}
	if v, at := data(api, `{}`); v != 4 || at != again {
		t.Fatalf("stored after fresh: %v %s", v, at)
	}
	// 7 days count fresh every time.
	create(api, `{"title": "PL68 fünf", "kind": "fault"}`)
	week := `{"from": "` + time.Now().Add(-7*24*time.Hour).UTC().Format(time.RFC3339) + `"}`
	if v, _ := data(api, week); v != 5 {
		t.Fatalf("7 days: %v", v)
	}
	// 30 days moving with the clock: a second later finds the same state.
	from := func(t time.Time) string { return `{"from": "` + t.UTC().Format(time.RFC3339) + `"}` }
	f0 := time.Now().Add(-30 * 24 * time.Hour)
	f1 := f0.Add(time.Second)
	_, at30 := data(api, from(f0))
	_, at := data(api, from(f1))
	if f0.Truncate(5*time.Minute).Equal(f1.Truncate(5*time.Minute)) && at != at30 {
		t.Fatalf("30 days a second later: %s, before %s, %d stored", at, at30, mem.len())
	}

	// The other person reads her own: her own stored state, not the admin's.
	create(viewer.api, `{"title": "PL68 ihrer", "kind": "wish"}`)
	if v, _ := data(viewer.api, `{}`); v != 1 {
		t.Fatalf("viewer: %v", v)
	}
	if v, _ := data(api, `{}`); v != 4 {
		t.Fatalf("admin after the viewer: %v", v)
	}

	// The stream of the dashboard shows the stored state.
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, api.URL+path+"/data", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, csrfValue)
	req.Header.Set("Cookie", sessionCookie+"="+api.cookie)
	res, err := api.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(raw), `"value":4`) || !strings.Contains(string(raw), `"computed_at":"`+again+`"`) {
		t.Fatalf("stream: %s, stored at %s", raw, again)
	}
}
