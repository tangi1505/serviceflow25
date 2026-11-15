// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// --- time limit of a request (PL56, Kern-Plan E69) ---
//
// Every API request ends after the time of ITSM_REQUEST_TIMEOUT (10 s unless set): its context
// runs out, pgx cancels the running statement in PostgreSQL, and the request answers 503.
// Found with H14: searches of ten seconds and more held every connection of the pool, and the
// writes and the jobs of the server waited behind them. The paths that may run longer stream
// or take large bodies: the export, the bulk stream, the event stream and the upload of files.

// longRunning reports whether a path is left without the time limit.
func longRunning(r *http.Request) bool {
	p := r.URL.Path
	return strings.HasSuffix(p, ":export") || strings.HasSuffix(p, ":bulk") ||
		p == "/api/v1/events/stream" || (p == "/api/v1/files" && r.Method == http.MethodPost)
}

// withDeadline gives every API request but the long-running ones a deadline; d <= 0 sets none.
func withDeadline(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if d <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") || longRunning(r) {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
