// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package scenario lists what is measured: the performance targets LZ1 to LZ13 (Hauptplan §2.5)
// and the infrastructure probes LP1 and LP2 (Lastprobe). A target becomes measurable when the
// task that builds its endpoint is done; until then it is reported as waiting, never skipped.
// A target measured before every part it depends on exists says so in its title.
package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Kind tells how a target is judged in the full stage.
type Kind string

const (
	// P95 means: the 95th percentile of latency must stay at or below Target (ms).
	P95 Kind = "p95_ms"
	// RPS means: throughput must reach at least Target (requests per second).
	RPS Kind = "rps"
	// Duration means: a whole run must finish within Target (seconds).
	Duration Kind = "duration_s"
	// None means: no absolute target, only compared with the baseline.
	None Kind = ""
)

// RequestFunc builds one request against a base URL.
type RequestFunc func(ctx context.Context, base string) (*http.Request, error)

// Scenario is one measurable thing.
type Scenario struct {
	ID     string
	Title  string
	Kind   Kind
	Target float64
	// WaitsFor names the tasks that must be built before the scenario can run.
	WaitsFor string
	// Request builds the measured request; nil while the scenario waits or needs Prepare.
	Request RequestFunc
	// Prepare creates what the scenario needs on the target (a session, a record to read) and
	// returns the request builder. It runs once per target and run.
	Prepare func(ctx context.Context, client *http.Client, base string) (RequestFunc, error)
	// Throughput is a run measured as a whole instead of single requests (LZ5, PL32): it sends
	// rows rows and returns how many were written; the measurement is rows per second.
	Throughput func(ctx context.Context, client *http.Client, base string, rows int) (int, error)
}

// Ready reports whether the scenario can run.
func (s Scenario) Ready() bool { return s.Request != nil || s.Prepare != nil || s.Throughput != nil }

func get(path string) RequestFunc {
	return func(ctx context.Context, base string) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	}
}

// probeType is the type of the Durchstich package (Hauptplan H13); the real packages replace it.
const probeType = "probe.probe"

// Account of the load tool (PL5): scripts/ci.sh creates it on the test database. Other
// environments set ITSM_LOAD_USER and ITSM_LOAD_PASSWORD.
const (
	defaultUser     = "load"
	defaultPassword = "Messlauf-Pw7!" //nolint:gosec // G101: the load account of the test database (scripts/ci.sh), no secret
	csrfHeader      = "X-Requested-With"
	csrfValue       = "itsm"
)

// session is a signed-in client for one target: cookie and CSRF header on every request.
type session struct {
	cookie string
}

// login signs in at the target and returns the session cookie.
func login(ctx context.Context, client *http.Client, base string) (session, error) {
	user, pw := os.Getenv("ITSM_LOAD_USER"), os.Getenv("ITSM_LOAD_PASSWORD")
	if user == "" {
		user = defaultUser
	}
	if pw == "" {
		pw = defaultPassword
	}
	body, _ := json.Marshal(map[string]string{"username": user, "password": pw})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return session{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, csrfValue)
	resp, err := client.Do(req)
	if err != nil {
		return session{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return session{}, fmt.Errorf("login as %s: %s: %s", user, resp.Status, strings.TrimSpace(string(raw)))
	}
	for _, c := range resp.Cookies() {
		if c.Name == "itsm_session" {
			return session{cookie: c.Name + "=" + c.Value}, nil
		}
	}
	return session{}, errors.New("login: no session cookie in the answer")
}

func (s session) apply(req *http.Request) {
	req.Header.Set("Cookie", s.cookie)
	req.Header.Set(csrfHeader, csrfValue)
}

func (s session) get(path string) RequestFunc {
	return func(ctx context.Context, base string) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return nil, err
		}
		s.apply(req)
		return req, nil
	}
}

func (s session) create(title, kind string, priority int) RequestFunc {
	return func(ctx context.Context, base string) (*http.Request, error) {
		body, _ := json.Marshal(map[string]any{"title": title, "kind": kind, "priority": priority})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/records/"+probeType, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		s.apply(req)
		return req, nil
	}
}

// createProbe posts one probe record and returns its id.
func (s session) createProbe(ctx context.Context, client *http.Client, base, title string) (string, error) {
	req, err := s.create(title, "fault", 2)(ctx, base)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create probe: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var rec struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil || rec.ID == "" {
		return "", errors.New("create probe: no id in response")
	}
	return rec.ID, nil
}

func prepareRead(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	id, err := s.createProbe(ctx, client, base, "load LZ1")
	if err != nil {
		return nil, err
	}
	return s.get("/api/v1/records/" + probeType + "/" + id), nil
}

func prepareList(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	// Make sure there is something to list; the data set in Prod size comes with PL31 to PL33.
	for i := range 60 {
		if _, err := s.createProbe(ctx, client, base, fmt.Sprintf("load LZ2 %d", i)); err != nil {
			return nil, err
		}
	}
	return s.get("/api/v1/records/" + probeType + "?limit=50&filter[kind]=fault&sort=-created_at"), nil
}

// searchWords are what LZ3 records are titled with and what it searches for.
var searchWords = []string{"Drucker", "Monitor", "Tastatur", "Netzwerk", "Telefon", "Server"}

func prepareSearch(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	// Something to find; the data set in Prod size comes with PL31 to PL33.
	for i := range 60 {
		title := fmt.Sprintf("load LZ3 %s defekt in Raum %d", searchWords[i%len(searchWords)], i)
		if _, err := s.createProbe(ctx, client, base, title); err != nil {
			return nil, err
		}
	}
	return s.get("/api/v1/search?q=Drucker%20defekt&limit=20"), nil
}

func prepareCreate(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	if err := s.loadRules(ctx, client, base); err != nil {
		return nil, err
	}
	return s.create("load LZ4", "wish", 3), nil
}

// loadRules puts the rules LZ4 writes with (WF4, Hauptplan §2.5 "mit Regeln"): eight before
// the write, each checked on every create, one of them matching (priority 3 sets tag_no), and
// two after the write as jobs (one notification, one that does not match). They only touch
// records titled "load LZ4", so the other scenarios stay as they were.
func (s session) loadRules(ctx context.Context, client *http.Client, base string) error {
	put := func(name string, rule map[string]any) error {
		body, _ := json.Marshal(rule)
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, base+"/api/v1/rules/"+name, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		s.apply(req)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("rule %s: %s: %s", name, resp.Status, strings.TrimSpace(string(raw)))
		}
		return nil
	}
	for i := range 8 {
		err := put(fmt.Sprintf("load_lz4_before_%d", i), map[string]any{
			"type_name": probeType, "trigger": "create", "mode": "before", "position": i,
			"condition": fmt.Sprintf(`record.title.startsWith("load LZ4") && record.priority == %d.0`, i),
			"actions":   []any{map[string]any{"set": map[string]any{"tag_no": fmt.Sprintf("LZ4-%d", i)}}},
		})
		if err != nil {
			return err
		}
	}
	for i, cond := range []string{`record.title.startsWith("load LZ4")`, `record.title.startsWith("load LZ4") && record.kind == "fault"`} {
		err := put(fmt.Sprintf("load_lz4_after_%d", i), map[string]any{
			"type_name": probeType, "trigger": "create", "mode": "after", "condition": cond,
			"actions": []any{map[string]any{"notify": map[string]any{"to": []string{"actors.assigned"}, "message": "LZ4"}}},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// All returns every scenario in report order.
func All() []Scenario {
	return []Scenario{
		{ID: "LP1", Title: "Health-Endpunkt (HTTP-Stapel ohne Abhängigkeiten)", Kind: None, Request: get("/healthz")},
		{ID: "LP2", Title: "Readiness (eine Abfrage an PostgreSQL, ein PING an Valkey)", Kind: None, Request: get("/readyz")},
		{ID: "LZ1", Title: "Einen Datensatz über die API lesen (mit Sitzung)", Kind: P95, Target: 50, Prepare: prepareRead},
		{ID: "LZ2", Title: "Liste mit 50 Zeilen, mit Filter (vorläufig ohne Rechte, bis PL8)", Kind: P95, Target: 150, Prepare: prepareList},
		{ID: "LZ3", Title: "Volltextsuche über alle lesbaren Typen", Kind: P95, Target: 300, Prepare: prepareSearch},
		{ID: "LZ4", Title: "Einen Datensatz schreiben, mit zehn Regeln (WF4)", Kind: P95, Target: 100, Prepare: prepareCreate},
		{ID: "LZ5", Title: "Bulk-Abgleich je Instanz (asynchron, probe.probe und core.person gemischt)", Kind: RPS, Target: 5000, Throughput: bulkRun},
		{ID: "LZ6", Title: "Phase 2 voll: 50.000 Computer mit je 150 Softwareeinträgen", Kind: Duration, Target: 300, WaitsFor: "PL32, PL33"},
		{ID: "LZ7", Title: "Phase 2 Delta: 1 % geändert", Kind: Duration, Target: 30, WaitsFor: "PL32, PL33"},
		{ID: "LZ8", Title: "Phase 1 voll: 4.600 Geräte", Kind: Duration, Target: 30, WaitsFor: "PL32, PL33"},
		{ID: "LZ9", Title: "Seitenwechsel in der Oberfläche", Kind: P95, Target: 500, WaitsFor: "UI2 (Messung mit Playwright)"},
		{ID: "LZ10", Title: "LZ1 bis LZ4 und LZ9 während eines Massenlaufs", Kind: P95, Target: 0, WaitsFor: "LZ1 bis LZ9"},
		{ID: "LZ11", Title: "Gesamtlast", Kind: RPS, Target: 2000, WaitsFor: "LZ1 bis LZ4"},
		{ID: "LZ12", Title: "Durchsatz wächst mit der Zahl der Instanzen", Kind: RPS, Target: 0, WaitsFor: "LZ11, Helm (BT)"},
		{ID: "LZ13", Title: "Verteiler: Bulk-Lauf, bis Webhooks und Regeln alle Ereignisse haben (PL41)", Kind: RPS, Target: 5000, Throughput: dispatchRun},
	}
}
