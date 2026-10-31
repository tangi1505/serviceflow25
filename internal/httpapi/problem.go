// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/jobs"
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
	"too_large":             {"de": "Anfrage zu groß", "en": "Request too large"},
	"unavailable":           {"de": "Nicht verfügbar", "en": "Unavailable"},
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
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
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
	case errors.Is(err, org.ErrInvalid), errors.Is(err, jobs.ErrInvalid):
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
