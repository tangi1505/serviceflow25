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

	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/relations"
)

// --- relations (PL15) ---

func (s *server) relationsReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Relations == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "relations are not configured on this instance", nil)
		return false
	}
	return true
}

func (s *server) ListRelationKinds(w http.ResponseWriter, r *http.Request) {
	if !s.relationsReady(w, r) {
		return
	}
	kinds, err := s.o.Relations.Kinds(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if kinds == nil {
		kinds = []meta.RelationKind{}
	}
	writeJSON(w, http.StatusOK, kinds)
}

func (s *server) CreateRelation(w http.ResponseWriter, r *http.Request) {
	if !s.relationsReady(w, r) {
		return
	}
	var body gen.RelationCreate
	if !decode(w, r, &body) {
		return
	}
	rel, err := s.o.Relations.Create(r.Context(), body.Kind, relations.End{Type: body.From.Type, ID: body.From.Id}, relations.End{Type: body.To.Type, ID: body.To.Id}, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/relations/"+rel.ID.String())
	writeJSON(w, http.StatusCreated, rel)
}

func (s *server) GetRelation(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if !s.relationsReady(w, r) {
		return
	}
	rel, err := s.o.Relations.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (s *server) DeleteRelation(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if !s.relationsReady(w, r) {
		return
	}
	s.noContent(w, r, s.o.Relations.Delete(r.Context(), id, actor(r)))
}

func (s *server) RecordRelations(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, p gen.RecordRelationsParams) {
	if !s.relationsReady(w, r) {
		return
	}
	kind, direction := "", ""
	if p.Kind != nil {
		kind = *p.Kind
	}
	if p.Direction != nil {
		direction = string(*p.Direction)
	}
	list, err := s.o.Relations.Of(r.Context(), relations.End{Type: typeName, ID: id}, kind, direction)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) RecordGraph(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, p gen.RecordGraphParams) {
	if !s.relationsReady(w, r) {
		return
	}
	walk := relations.Walk{}
	if p.Kinds != nil && *p.Kinds != "" {
		walk.Kinds = strings.Split(*p.Kinds, ",")
	}
	if p.Direction != nil {
		walk.Direction = string(*p.Direction)
	}
	if p.Depth != nil {
		walk.Depth = *p.Depth
	}
	g, err := s.o.Relations.Graph(r.Context(), relations.End{Type: typeName, ID: id}, walk)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *server) RecordImpact(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, p gen.RecordImpactParams) {
	if !s.relationsReady(w, r) {
		return
	}
	depth := 0
	if p.Depth != nil {
		depth = *p.Depth
	}
	g, err := s.o.Relations.Impact(r.Context(), relations.End{Type: typeName, ID: id}, depth)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}
