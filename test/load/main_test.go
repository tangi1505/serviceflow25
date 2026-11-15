// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"group24.de/itsm/test/load/internal/gate"
)

// fakeITSM answers like the API as far as the scenarios need: creating a record gives 201
// with an id, a bulk stream 202 with a job that is done at once (LZ5), everything else 200.
func fakeITSM() *httptest.Server {
	var (
		mu   sync.Mutex
		runs []int // rows per bulk job; the job id is the index plus one
	)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":bulk") {
			raw, _ := io.ReadAll(r.Body)
			mu.Lock()
			runs = append(runs, bytes.Count(raw, []byte("\n")))
			id := len(runs)
			mu.Unlock()
			w.Header().Set("Location", fmt.Sprintf("/api/v1/jobs/%d", id))
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"id":%d,"kind":"bulk.apply"}`, id)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/events/consumers" {
			// The fake has no dispatcher: nothing is ever behind (LZ13).
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/jobs/") {
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/"))
			mu.Lock()
			n := runs[id-1]
			mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"id":%d,"state":"completed","output":{"done":true,"created":%d}}`, id, n)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "itsm_session", Value: "fake", Path: "/"})
			_, _ = w.Write([]byte(`{"id":"01a15a3c-8545-7691-b780-9b915e555b1c","username":"load"}`))
			return
		}
		// LZ14 to LZ17 (H14): an export of 10,000 rows, the pages of a query, a batch whose
		// rows all succeed, and the person of the account.
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":export") {
			w.Header().Set("ITSM-Rows", "10000")
			_, _ = w.Write([]byte("title\n"))
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":query") {
			raw, _ := io.ReadAll(r.Body)
			items := make([]string, 500)
			for i := range items {
				items[i] = fmt.Sprintf(`{"id":"01a15a3c-8545-7691-b780-%012d","version":1}`, i)
			}
			next := `"p2"`
			if bytes.Contains(raw, []byte(`"after"`)) {
				next = `""`
			}
			_, _ = fmt.Fprintf(w, `{"items":[%s],"next":%s}`, strings.Join(items, ","), next)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batch") {
			var in struct {
				Rows []struct {
					ID string `json:"id"`
				} `json:"rows"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			res := make([]string, len(in.Rows))
			for i, row := range in.Rows {
				res[i] = fmt.Sprintf(`{"id":%q,"status":"ok","version":2}`, row.ID)
			}
			_, _ = fmt.Fprintf(w, `{"results":[%s]}`, strings.Join(res, ","))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me" {
			_, _ = w.Write([]byte(`{"person_id":"01a15a3c-8545-7691-b780-9b915e555b1d"}`))
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/api/v1/records/") {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"01a15a3c-8545-7691-b780-9b915e555b1c","version":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
}

// The whole chain in A/B mode: the same server as new build and as baseline must pass; the
// new build behind a slowing proxy must fail. This is the self-test from the Prüfweg of H11.
func TestGateCatchesInjectedDelayInAB(t *testing.T) {
	if testing.Short() {
		t.Skip("takes a few seconds")
	}
	t.Setenv("ITSM_LOAD_INJECT_DELAY", "")
	t.Setenv("ITSM_LOAD_IGNORE_LOAD", "1") // the machine running the tests may be busy
	srv := fakeITSM()
	defer srv.Close()
	dir := t.TempDir()
	same, slow := filepath.Join(dir, "same.json"), filepath.Join(dir, "slow.json")

	old := stages["quick"]
	stages["quick"] = stage{workers: 4, warmup: 200 * time.Millisecond, duration: 500 * time.Millisecond, repeat: 4, bulkRows: 2000}
	defer func() { stages["quick"] = old }()
	// This test checks the chain, not the noise level: under -race and next to other packages'
	// tests, two identical servers differed by up to 1.1 ms in p95 (CI run of 2026-10-20 after
	// PL5). The real noise is measured in the CI A/B run (Hauptplan H11, PL2). A 20 ms brake is
	// far above this floor. The throughput of the bulk against the fake server in these short
	// runs scatters more: the best of four runs differed by 11 % between identical servers (CI
	// run of 2026-10-29 after UI10, single runs 17,000 to 32,000 rows/s), so 50 % here; the brake
	// still fails on p95 (about 21 ms against 1 ms).
	oldRules := quickRules
	quickRules = gate.Rules{MaxSlowdown: 0.50, FloorMs: 5.0}
	defer func() { quickRules = oldRules }()

	var out bytes.Buffer
	must := func(args ...string) {
		t.Helper()
		if err := run(args, &out); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
	}
	must("run", "--stage=quick", "--target="+srv.URL, "--baseline-target="+srv.URL, "--out="+same)
	must("gate", "--stage=quick", "--result="+same, "--baseline="+filepath.Join(dir, "none.json"))
	must("run", "--stage=quick", "--target="+srv.URL, "--baseline-target="+srv.URL, "--out="+slow, "--inject-delay=20ms")

	out.Reset()
	err := run([]string{"gate", "--stage=quick", "--result=" + slow, "--baseline=" + filepath.Join(dir, "none.json")}, &out)
	if err == nil || !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("gate must fail with 20 ms extra delay on the new build only:\n%s", out.String())
	}
}

// A baseline build without the records endpoint (404 on create) is not measured: no busy loop
// of failing calls, a note in the result, and the gate reports the new values without comparing.
func TestBaselineWithoutEndpointIsReportedNotMeasured(t *testing.T) {
	if testing.Short() {
		t.Skip("takes a second")
	}
	t.Setenv("ITSM_LOAD_IGNORE_LOAD", "1")
	srv := fakeITSM()
	defer srv.Close()
	older := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/records/") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer older.Close()
	old := stages["quick"]
	stages["quick"] = stage{workers: 2, warmup: 50 * time.Millisecond, duration: 200 * time.Millisecond, repeat: 2, bulkRows: 2000}
	defer func() { stages["quick"] = old }()
	// This test checks the "unavailable" path, not the noise level (see the A/B test above).
	oldRules := quickRules
	quickRules = gate.Rules{MaxSlowdown: 0.10, FloorMs: 5.0}
	defer func() { quickRules = oldRules }()
	file := filepath.Join(t.TempDir(), "r.json")
	var out bytes.Buffer
	if err := run([]string{"run", "--stage=quick", "--target=" + srv.URL, "--baseline-target=" + older.URL, "--out=" + file}, &out); err != nil {
		t.Fatal(err)
	}
	var rep gate.Report
	raw, _ := os.ReadFile(file)
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	b := rep.Baseline["LZ1"]
	if b.Unavailable == "" || b.Requests != 0 {
		t.Fatalf("LZ1 at the baseline must be unavailable without requests, got %+v", b)
	}
	if !strings.Contains(out.String(), "LZ1   Basislinie nicht messbar") {
		t.Fatalf("note missing:\n%s", out.String())
	}
	out.Reset()
	if err := run([]string{"gate", "--stage=quick", "--result=" + file, "--baseline=/nonexistent"}, &out); err != nil {
		t.Fatalf("gate must pass: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "beherrscht das Szenario noch nicht") {
		t.Fatalf("gate note missing:\n%s", out.String())
	}
}

func TestRunWithoutBaselineOnlyReports(t *testing.T) {
	if testing.Short() {
		t.Skip("takes a second")
	}
	t.Setenv("ITSM_LOAD_IGNORE_LOAD", "1")
	srv := fakeITSM()
	defer srv.Close()
	old := stages["quick"]
	stages["quick"] = stage{workers: 2, warmup: 50 * time.Millisecond, duration: 200 * time.Millisecond, repeat: 1, bulkRows: 2000}
	defer func() { stages["quick"] = old }()
	file := filepath.Join(t.TempDir(), "r.json")
	var out bytes.Buffer
	if err := run([]string{"run", "--stage=quick", "--target=" + srv.URL, "--out=" + file}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"gate", "--stage=quick", "--result=" + file, "--baseline=/nonexistent"}, &out); err != nil {
		t.Fatalf("without a baseline the gate must pass with a note: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "keine Basislinie") {
		t.Fatalf("note missing:\n%s", out.String())
	}
}

func TestBaselineStoresBinaryAndRecord(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "itsm")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho itsm\n"), 0o750); err != nil {
		t.Fatal(err)
	}
	res := filepath.Join(dir, "r.json")
	if err := os.WriteFile(res, []byte(`{"stage":"quick","results":{"LP1":{"p95_ms":1}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(dir, "store")
	rec := filepath.Join(dir, "baseline.json")
	var out bytes.Buffer
	if err := run([]string{"baseline", "--result=" + res, "--binary=" + bin, "--store=" + store, "--out=" + rec, "--note=test"}, &out); err != nil {
		t.Fatal(err)
	}
	want, _ := sha256File(bin)
	got, err := sha256File(filepath.Join(store, BaselineBinary))
	if err != nil || got != want {
		t.Fatalf("stored binary differs: %v %s %s", err, got, want)
	}
	b, _ := os.ReadFile(rec)
	if !strings.Contains(string(b), want) || !strings.Contains(string(b), `"note": "test"`) {
		t.Fatalf("record incomplete: %s", b)
	}
	if err := run([]string{"baseline", "--result=" + res, "--binary=" + bin}, &out); err == nil {
		t.Fatal("expected an error without --note")
	}
}

func TestGenerateTiny(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"generate", "--size=tiny", "--out=" + t.TempDir()}, &out); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"persons", "computers", "hypervisors", "switches", "tickets"} {
		if !strings.Contains(out.String(), kind) {
			t.Errorf("%s missing in output:\n%s", kind, out.String())
		}
	}
}
