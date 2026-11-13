// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package httpapi is the HTTP entry point: router, middleware, the generated API routes, the
// health endpoints and the static web interface (architektur.md §2 "HTTP"; §6).
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/bulk"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/calendar"
	"group24.de/itsm/internal/customer"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/export"
	"group24.de/itsm/internal/files"
	"group24.de/itsm/internal/forms"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/portal"
	"group24.de/itsm/internal/prefs"
	"group24.de/itsm/internal/presence"
	"group24.de/itsm/internal/query"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/relations"
	"group24.de/itsm/internal/rules/engine"
	"group24.de/itsm/internal/search"
	"group24.de/itsm/internal/seclog"
	"group24.de/itsm/internal/settings"
	"group24.de/itsm/internal/sla"
	"group24.de/itsm/internal/snippets"
	"group24.de/itsm/internal/webhooks"
)

// Check is one dependency that /readyz verifies.
type Check struct {
	Name string
	Fn   func(context.Context) error
}

// Options configure the router.
type Options struct {
	Logger  *slog.Logger
	Version string
	Checks  []Check
	// API enables /api/v1 and the web interface; a pure worker serves health endpoints only.
	API bool
	// Records is required when API is set.
	Records *records.Service
	// Auth signs accounts in; without it every protected call answers 401.
	Auth *auth.Service
	// Org is the organisation tree (PL6); required when API is set.
	Org *org.Service
	// Settings knows the declared settings and checks values (SD20); nil stores any key.
	Settings *settings.Service
	// Groups answers the membership of groups (PL7); required when API is set.
	Groups *query.Groups
	// Authz loads the rights of the session (PL8); without it a session may do nothing.
	Authz *authz.Service
	// Jobs answers /api/v1/jobs (PL23); without it those endpoints answer 503.
	Jobs *jobs.Service
	// Events is the stream (PL24); without it /api/v1/events answers 503.
	Events *events.Service
	// Authn signs people in through an identity provider and keeps the login rules (PL11);
	// nil means no provider and no rules.
	Authn *authn.Service
	// TrustProxy takes the caller's address from X-Forwarded-For (PL13).
	TrustProxy bool
	// Relations links records (PL15); nil answers 503 on the relation endpoints.
	Relations *relations.Service
	// Files keeps files and attachments (PL18); nil answers 503 on the file endpoints.
	Files *files.Service
	// Webhooks keeps the subscriptions (PL25); nil answers 503 on the webhook endpoints.
	Webhooks *webhooks.Service
	// Search is the full-text search (PL26, E21); nil answers 503.
	Search search.Engine
	// SecLog is the security event log (PL27); nil records nothing and answers 503.
	SecLog *seclog.Log
	// KV keeps the answers to Idempotency-Key of the bulk (PL31); nil answers 503 to a bulk
	// call with that header.
	KV *cache.Client
	// Bulk keeps NDJSON streams and queues their jobs (PL32); nil answers 503 to a stream.
	Bulk *bulk.Async
	// Export writes lists as CSV, XLSX or ODS (UI5); nil answers 503.
	Export *export.Service
	// Presence keeps who has a record open (UI8); nil answers 503.
	Presence *presence.Service
	// Prefs keeps one's own settings (UI9); nil answers 401.
	Prefs *prefs.Service
	// Forms offers and submits the forms (UI11); nil answers 503.
	Forms *forms.Service
	// Portal reads the notices under "Aktuell" (SD5); nil answers 503.
	Portal *portal.Service
	// Snippets fills in the text modules (SD8); nil answers 503.
	Snippets *snippets.Service
	// Customer keeps the customer layer of the types (PL43); nil answers 503.
	Customer *customer.Service
	// Rules keeps the rules "if, then" (WF4); nil answers 503 on /api/v1/rules.
	Rules *engine.Engine
	// Calendars count working time (WF8); nil answers 503 on /api/v1/calendars.
	Calendars *calendar.Service
	// SLA moves the clocks of the records (WF9); nil answers 503 on …/clocks.
	SLA *sla.Service
	// WebDir is the directory of the built web interface; empty disables it.
	WebDir string
}

// NewRouter builds the HTTP handler.
func NewRouter(o Options) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, requestLogger(o.Logger))

	if !o.API {
		r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})
		r.Get("/readyz", readiness(o.Checks, o.Logger))
		return r
	}

	s := &server{o: o}
	r.Use(authenticate(o), withLanguage)
	gen.HandlerWithOptions(s, gen.ChiServerOptions{
		BaseRouter: r,
		// Bad path or query parameters (an id that is no UUID, a limit that is no number).
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", err.Error(), nil)
		},
	})
	r.Route("/api", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "no such endpoint", nil)
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			writeProblem(w, r, http.StatusMethodNotAllowed, "bad_request", "method not allowed", nil)
		})
	})
	if o.WebDir != "" {
		r.NotFound(spa(os.DirFS(o.WebDir)))
	}
	return r
}

func readiness(checks []Check, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		status := http.StatusOK
		result := make(map[string]string, len(checks))
		for _, c := range checks {
			if err := c.Fn(ctx); err != nil {
				// Details go to the log, not to the caller.
				logger.Warn("readiness check failed", "check", c.Name, "error", err.Error())
				result[c.Name] = "fail"
				status = http.StatusServiceUnavailable
				continue
			}
			result[c.Name] = "ok"
		}
		writeJSON(w, status, result)
	}
}

// spa serves files of the web interface and falls back to index.html for client routes.
// os.DirFS with fs.ValidPath keeps requests inside the directory.
func spa(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || !fs.ValidPath(name) {
			name = "index.html"
		}
		if info, err := fs.Stat(fsys, name); err != nil || info.IsDir() {
			name = "index.html"
		}
		// name passed fs.ValidPath and fsys is an os.DirFS rooted at the web directory, so it
		// cannot leave it; router_test.go TestWebInterface checks this.
		http.ServeFileFS(w, r, fsys, name) //nolint:gosec // G703: confined by fs.ValidPath and os.DirFS
	}
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", float64(time.Since(start).Microseconds())/1000,
				"request_id", middleware.GetReqID(r.Context()))
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Serve runs the HTTP server until ctx is cancelled, then shuts down gracefully.
func Serve(ctx context.Context, addr string, h http.Handler, shutdown time.Duration, logger *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errs := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errs <- err
	}()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	logger.Info("http server shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), shutdown)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return err
	}
	return <-errs
}
