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
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The time limit of a request (PL56): an API request gets a deadline, the export, the bulk
// stream, the event stream and the upload of files do not; a statement cut off by it answers
// 503 with the problem "timeout".
func TestRequestTimeLimit(t *testing.T) {
	var got []string
	h := withDeadline(time.Second)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, has := r.Context().Deadline()
		got = append(got, fmt.Sprintf("%s %s %v", r.Method, r.URL.Path, has))
	}))
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/records/incident.incident:query"},
		{http.MethodGet, "/api/v1/search"},
		{http.MethodPost, "/api/v1/records/incident.incident:export"},
		{http.MethodPost, "/api/v1/records/core.person:bulk"},
		{http.MethodGet, "/api/v1/events/stream"},
		{http.MethodPost, "/api/v1/files"},
		{http.MethodGet, "/api/v1/files/0190a8d0-0000-7000-8000-000000000000"},
		{http.MethodGet, "/assets/index.js"},
	} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), c.method, c.path, nil))
	}
	want := []string{
		"POST /api/v1/records/incident.incident:query true",
		"GET /api/v1/search true",
		"POST /api/v1/records/incident.incident:export false",
		"POST /api/v1/records/core.person:bulk false",
		"GET /api/v1/events/stream false",
		"POST /api/v1/files false",
		"GET /api/v1/files/0190a8d0-0000-7000-8000-000000000000 true",
		"GET /assets/index.js false",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("deadlines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	rec := httptest.NewRecorder()
	cut := fmt.Errorf("records: query incident.incident: %w", fmt.Errorf("timeout: %w", context.DeadlineExceeded))
	writeError(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/search", nil), slog.New(slog.NewTextHandler(io.Discard, nil)), cut)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"/problems/timeout"`) {
		t.Fatalf("a cut-off statement: %d %s", rec.Code, rec.Body.String())
	}
}
