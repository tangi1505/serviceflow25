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

	"group24.de/itsm/internal/authz"
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
	ctx := r.Context()
	if body.Comment != nil {
		// The text of the transition, an entry of the same write (SD-Plan N8).
		ctx = records.WithComment(ctx, *body.Comment)
	}
	rec, err := s.o.Records.Transition(ctx, typeName, id, body.Transition, fields, version, actor(r))
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
	if id == nil {
		writeJSON(w, http.StatusOK, s.withNodes(r, typeName, fs))
		return
	}
	writeJSON(w, http.StatusOK, fs)
}

// draftStates are the field states of a new record with where it may go (PL46): the node it
// lands in without a choice, empty when the person has to name one, and every node allowed.
type draftStates struct {
	records.FieldStates
	Node  string    `json:"node"`
	Nodes []nodeRef `json:"nodes"`
}

type nodeRef struct {
	Path  string            `json:"path"`
	Label map[string]string `json:"label"`
}

func (s *server) withNodes(r *http.Request, typeName string, fs records.FieldStates) draftStates {
	a := authz.From(r.Context())
	out := draftStates{FieldStates: fs, Nodes: []nodeRef{}}
	out.Node, _ = a.CreateNode(typeName)
	reach := a.CreateReach(typeName)
	if len(reach) == 0 || s.o.Org == nil {
		return out
	}
	nodes, err := s.o.Org.Nodes(r.Context())
	if err != nil {
		s.o.Logger.Warn("nodes for a new record", "err", err)
		return out
	}
	for _, n := range nodes {
		for _, rc := range reach {
			if rc.Covers(n.Path) {
				out.Nodes = append(out.Nodes, nodeRef{Path: n.Path, Label: n.Label})
				break
			}
		}
	}
	return out
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
