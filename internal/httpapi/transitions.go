// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/records"
)

// --- state model (WF1), field states (WF2) and the event stream (E39) ---

func (s *server) TransitionRecord(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, p gen.TransitionRecordParams) {
	var body gen.TransitionRequest
	if !decode(w, r, &body) {
		return
	}
	// Fields go with If-Match like a PATCH (E29); a bare transition may name the version.
	version := int64(-1)
	hasMatch := p.IfMatch != nil && strings.TrimSpace(*p.IfMatch) != ""
	if body.Fields != nil && !hasMatch {
		writeProblem(w, r, http.StatusPreconditionRequired, "precondition_required",
			"send If-Match with the ETag of the record as you last saw it when the transition sets fields", nil)
		return
	}
	if hasMatch {
		v, err := parseETag(*p.IfMatch)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "If-Match must be the ETag of the record, e.g. \"3\"", nil)
			return
		}
		version = v
	}
	var fields map[string]any
	if body.Fields != nil {
		fields = *body.Fields
	}
	rec, err := s.o.Records.Transition(r.Context(), typeName, id, body.Transition, fields, version, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if !s.localize(w, r, typeName, rec) {
		return
	}
	w.Header().Set("ETag", etag(rec))
	writeJSON(w, http.StatusOK, rec)
}

func (s *server) RecordFieldStates(w http.ResponseWriter, r *http.Request, typeName gen.TypeName) {
	var body gen.FieldStatesRequest
	if !decode(w, r, &body) {
		return
	}
	var id *uuid.UUID
	if body.Id != nil {
		id = body.Id
	}
	var draft map[string]any
	if body.Values != nil {
		draft = *body.Values
	}
	fs, err := s.o.Records.FieldStates(r.Context(), typeName, id, draft)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, fs)
}

func (s *server) RecordTransitions(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	infos, err := s.o.Records.Transitions(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if infos == nil {
		infos = []records.TransitionInfo{}
	}
	writeJSON(w, http.StatusOK, infos)
}
