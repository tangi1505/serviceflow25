// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/presence"
)

// --- who has a record open (UI8, Hauptplan E130) ---

func (s *server) presenceReady(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	if s.o.Presence == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "presence needs Valkey", nil)
		return auth.Session{}, false
	}
	sess, ok := auth.SessionFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "presence needs a signed-in session", nil)
		return auth.Session{}, false
	}
	return sess, true
}

func (s *server) BeatPresence(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	sess, ok := s.presenceReady(w, r)
	if !ok {
		return
	}
	// Only who may read the record says to be on it, and learns who else is.
	if _, err := s.o.Records.Get(r.Context(), typeName, id); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	var body struct {
		Editing bool `json:"editing"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil && err != io.EOF {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "body must be {\"editing\": true|false}", nil)
		return
	}
	name := sess.DisplayName
	if name == "" {
		name = sess.Username
	}
	others, err := s.o.Presence.Beat(r.Context(), typeName, id, sess.AccountID, name, body.Editing)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if others == nil {
		others = []presence.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"others": others})
}

func (s *server) LeavePresence(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	sess, ok := s.presenceReady(w, r)
	if !ok {
		return
	}
	// A record the session may not read is not there, not even to leave it (PL44).
	if _, err := s.o.Records.Get(r.Context(), typeName, id); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.noContent(w, r, s.o.Presence.Leave(r.Context(), typeName, id, sess.AccountID))
}
