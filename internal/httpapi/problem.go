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
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/notify"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/query"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/relations"
)

// problem is an RFC 9457 problem details document (Hauptplan E71).
type problem struct {
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Status   int               `json:"status"`
	Detail   string            `json:"detail,omitempty"`
	Instance string            `json:"instance,omitempty"`
	Errors   []records.Problem `json:"errors,omitempty"`
}

// titles per problem code and language; the code is also the last part of the type URI.
var titles = map[string]map[string]string{
	"bad_request":           {"de": "Ungültige Anfrage", "en": "Bad request"},
	"not_found":             {"de": "Nicht gefunden", "en": "Not found"},
	"conflict":              {"de": "Konflikt", "en": "Conflict"},
	"validation":            {"de": "Eingabe ungültig", "en": "Validation failed"},
	"precondition_required": {"de": "If-Match fehlt", "en": "If-Match required"},
	"unauthorized":          {"de": "Nicht angemeldet", "en": "Not signed in"},
	"forbidden":             {"de": "Nicht erlaubt", "en": "Forbidden"},
	"invalid_credentials":   {"de": "Anmeldung fehlgeschlagen", "en": "Sign-in failed"},
	"password_expired":      {"de": "Kennwort abgelaufen", "en": "Password expired"},
	"internal":              {"de": "Interner Fehler", "en": "Internal error"},
	"cancelled":             {"de": "Abgebrochen", "en": "Cancelled"},
	"too_large":             {"de": "Anfrage zu groß", "en": "Request too large"},
	"unavailable":           {"de": "Nicht verfügbar", "en": "Unavailable"},
	"locked":                {"de": "Zu viele Versuche", "en": "Too many attempts"},
	"busy":                  {"de": "Überlastet", "en": "Busy"},
}

// retryAfter sets Retry-After in whole seconds, at least one.
func retryAfter(w http.ResponseWriter, d time.Duration) {
	secs := int((d + time.Second - 1) / time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
}

// writeProblem answers with application/problem+json.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string, errs []records.Problem) {
	p := problem{
		Type:     "/problems/" + code,
		Title:    titles[code][pickLanguage(r.Header.Get("Accept-Language"))],
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
		Errors:   errs,
	}
	if p.Title == "" {
		p.Title = http.StatusText(status)
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// writeError maps the errors of the records package to problems; anything else is 500 and logged.
// statusClientClosed is the status of a request the client cancelled (nginx: 499).
const statusClientClosed = 499

func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		// The time limit of the request (PL56): the statement was cancelled in PostgreSQL.
		logger.Warn("request took too long", "method", r.Method, "path", r.URL.Path)
		writeProblem(w, r, http.StatusServiceUnavailable, "timeout", "the request took longer than the server allows; narrow it down", nil)
	case errors.Is(err, context.Canceled) && r.Context().Err() != nil:
		// The client went away, such as a dashboard whose period changed (PL65): no error of the
		// server; the request log shows it with 499 as nginx does, and nobody reads the answer.
		logger.Info("request cancelled by the client", "method", r.Method, "path", r.URL.Path)
		writeProblem(w, r, statusClientClosed, "cancelled", "the client cancelled the request", nil)
	case errors.Is(err, auth.ErrBusy):
		// Too many password checks at once in this instance (AP6): come back in a second.
		retryAfter(w, time.Second)
		writeProblem(w, r, http.StatusServiceUnavailable, "busy", "too many sign-ins at once; try again in a second", nil)
	case errors.Is(err, records.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, records.ErrConflict), errors.Is(err, records.ErrNotAllowed),
		errors.Is(err, org.ErrExists), errors.Is(err, org.ErrNotEmpty):
		writeProblem(w, r, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, records.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "forbidden", err.Error(), nil)
	case errors.Is(err, authz.ErrExists), errors.Is(err, authz.ErrInUse):
		writeProblem(w, r, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, authz.ErrInvalid), errors.Is(err, authz.ErrNoPerson):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", err.Error(), nil)
	case errors.Is(err, org.ErrNotFound), errors.Is(err, query.ErrNotFound), errors.Is(err, authz.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, org.ErrInvalid), errors.Is(err, jobs.ErrInvalid), errors.Is(err, notify.ErrInvalid):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", err.Error(), nil)
	case errors.Is(err, jobs.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, events.ErrGone):
		writeProblem(w, r, http.StatusGone, "gone", err.Error(), nil)
	case errors.Is(err, authn.ErrInvalid):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", err.Error(), nil)
	case errors.Is(err, authn.ErrNotFound), errors.Is(err, relations.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, relations.ErrInvalid):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", err.Error(), nil)
	default:
		if ve, ok := records.AsValidationError(err); ok {
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", ve.Problems)
			return
		}
		logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err.Error())
		writeProblem(w, r, http.StatusInternalServerError, "internal", "", nil)
	}
}
