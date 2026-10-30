// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"

	"group24.de/itsm/internal/httpapi/gen"
)

// --- forms (UI11, Hauptplan E133) ---

// forms answers 503 when the server runs without forms.
func (s *server) forms(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Forms == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "forms are not configured", nil)
		return false
	}
	return true
}

func (s *server) ListForms(w http.ResponseWriter, r *http.Request) {
	if !s.forms(w, r) {
		return
	}
	list, err := s.o.Forms.List(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) GetForm(w http.ResponseWriter, r *http.Request, formID gen.FormId) {
	if !s.forms(w, r) {
		return
	}
	f, err := s.o.Forms.Get(r.Context(), formID)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *server) SubmitForm(w http.ResponseWriter, r *http.Request, formID gen.FormId) {
	if !s.forms(w, r) {
		return
	}
	input, ok := readBody(w, r)
	if !ok {
		return
	}
	answers, _ := input["answers"].(map[string]any)
	if answers == nil {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "answers must be an object of the answers per question id", nil)
		return
	}
	rec, err := s.o.Forms.Submit(r.Context(), formID, answers, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}
