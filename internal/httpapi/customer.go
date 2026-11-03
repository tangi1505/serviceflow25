// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/rules"
)

// --- customer layer (PL43, Hauptplan E135) ---

// typeRights is the pseudo type of the rights on the definitions: customize changes types in
// the customer layer. The types belong to the installation (architektur.md §5.1).
const typeRights = "core.type"

// mayCustomize answers 503 without the service and 403 without the right customize.
func (s *server) mayCustomize(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Customer == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "the customer layer is not configured", nil)
		return false
	}
	// The types belong to the installation (PL44): a change reaches every tenant, so the
	// right comes from a role at the root.
	if !authz.From(r.Context()).Installation(typeRights, "customize") {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "changing types needs customize on "+typeRights+" at the root", nil)
		return false
	}
	return true
}

func (s *server) GetCustomerLayer(w http.ResponseWriter, r *http.Request, typeName gen.TypeName) {
	if !s.mayCustomize(w, r) {
		return
	}
	v, err := s.o.Customer.Get(r.Context(), typeName)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) AddCustomerChange(w http.ResponseWriter, r *http.Request, typeName gen.TypeName) {
	if !s.mayCustomize(w, r) {
		return
	}
	var body struct {
		Version int64       `json:"version"`
		Change  meta.Change `json:"change"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&body); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "the body must be {\"version\": n, \"change\": {...}}: "+err.Error(), nil)
		return
	}
	var by uuid.UUID
	if a := actor(r); a != nil {
		by = *a
	}
	v, err := s.o.Customer.Add(r.Context(), typeName, body.Change, body.Version, by)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) RemoveCustomerChange(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, changeID string, params gen.RemoveCustomerChangeParams) {
	if !s.mayCustomize(w, r) {
		return
	}
	v, err := s.o.Customer.Remove(r.Context(), typeName, changeID, params.Version)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// CheckCondition says whether a CEL condition over a record compiles (UI26.1): the editor
// of the workflow asks while one types.
func (s *server) CheckCondition(w http.ResponseWriter, r *http.Request, typeName gen.TypeName) {
	if !s.mayCustomize(w, r) {
		return
	}
	var body gen.ConditionCheck
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	if err := dec.Decode(&body); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "the body must be {\"when\": \"…\"}: "+err.Error(), nil)
		return
	}
	out := gen.ConditionResult{Ok: true}
	if body.When != "" {
		if _, err := rules.Compile(body.When); err != nil {
			msg := err.Error()
			out.Ok, out.Error = false, &msg
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) ExportCustomerLayers(w http.ResponseWriter, r *http.Request) {
	if !s.mayCustomize(w, r) {
		return
	}
	list, err := s.o.Customer.Export(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
