// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func get(t *testing.T, h http.Handler, target string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	var body map[string]any
	if ct := rec.Header().Get("Content-Type"); strings.HasPrefix(ct, "application/json") || strings.HasPrefix(ct, "application/problem+json") {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: invalid JSON %q", target, rec.Body.String())
		}
	}
	return rec, body
}

func ok(context.Context) error   { return nil }
func fail(context.Context) error { return errors.New("down") }

func TestHealthz(t *testing.T) {
	h := NewRouter(Options{Logger: quiet})
	rec, body := get(t, h, "/healthz")
	if rec.Code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}

func TestReadyzAllOK(t *testing.T) {
	h := NewRouter(Options{Logger: quiet, Checks: []Check{{"database", ok}, {"valkey", ok}}})
	rec, body := get(t, h, "/readyz")
	if rec.Code != http.StatusOK || body["database"] != "ok" || body["valkey"] != "ok" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}

func TestReadyzReportsFailureWithoutDetails(t *testing.T) {
	h := NewRouter(Options{Logger: quiet, Checks: []Check{{"database", ok}, {"valkey", fail}}})
	rec, body := get(t, h, "/readyz")
	if rec.Code != http.StatusServiceUnavailable || body["valkey"] != "fail" || body["database"] != "ok" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	if strings.Contains(rec.Body.String(), "down") {
		t.Fatal("error details must not reach the caller")
	}
}

func TestVersionAndAPINotFound(t *testing.T) {
	h := NewRouter(Options{Logger: quiet, Version: "1.2.3", API: true})
	rec, body := get(t, h, "/api/v1/version")
	if rec.Code != http.StatusOK || body["version"] != "1.2.3" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	// Without a session nothing under /api/v1 but version and login answers, not even 404
	// (E78); the 404 as problem is tested with a session in api_test.go.
	rec, body = get(t, h, "/api/v1/nothing")
	if rec.Code != http.StatusUnauthorized || body["type"] != "/problems/unauthorized" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}

func TestWorkerHasNoAPI(t *testing.T) {
	h := NewRouter(Options{Logger: quiet, Version: "1.2.3", API: false})
	if rec, _ := get(t, h, "/api/v1/version"); rec.Code != http.StatusNotFound {
		t.Fatalf("worker answered /api/v1/version with %d", rec.Code)
	}
}

func TestWebInterface(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", "INDEX")
	write("assets/app.js", "JS")
	secret := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(secret) })

	h := NewRouter(Options{Logger: quiet, API: true, WebDir: dir})
	cases := map[string]string{
		"/":              "INDEX",
		"/assets/app.js": "JS",
		"/tickets/42":    "INDEX",
		"/assets":        "INDEX",
	}
	for target, want := range cases {
		rec, _ := get(t, h, target)
		if rec.Code != http.StatusOK || rec.Body.String() != want {
			t.Errorf("%s: got %d %q, want %q", target, rec.Code, rec.Body.String(), want)
		}
	}
	// Paths that try to leave the directory must never return the file outside.
	for _, target := range []string{"/../secret.txt", "/%2e%2e/secret.txt", "/assets/../../secret.txt"} {
		rec, _ := get(t, h, target)
		if strings.Contains(rec.Body.String(), "SECRET") {
			t.Errorf("%s: served a file outside the web directory", target)
		}
		if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 200 or 400", target, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/tickets", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST to the web interface: got %d", rec.Code)
	}
}
