// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authz"
)

// --- one's own settings (UI9, Hauptplan E131) ---

// me returns the account and person of the session, or answers 401.
func (s *server) me(w http.ResponseWriter, r *http.Request) (uuid.UUID, *uuid.UUID, bool) {
	sess, ok := auth.SessionFrom(r.Context())
	if !ok || s.o.Prefs == nil {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "settings need a signed-in session", nil)
		return uuid.UUID{}, nil, false
	}
	return sess.AccountID, authz.From(r.Context()).PersonID, true
}

func (s *server) MySettings(w http.ResponseWriter, r *http.Request) {
	account, person, ok := s.me(w, r)
	if !ok {
		return
	}
	p, err := s.o.Prefs.Get(r.Context(), account, person)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) SetMySettings(w http.ResponseWriter, r *http.Request) {
	account, person, ok := s.me(w, r)
	if !ok {
		return
	}
	input, ok := readBody(w, r)
	if !ok {
		return
	}
	if err := s.o.Prefs.Set(r.Context(), account, person, input); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.MySettings(w, r)
}
