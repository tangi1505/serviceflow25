// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"group24.de/itsm/internal/httpapi/gen"
)

// --- costs and times (PL20, KF18, Hauptplan E97) ---

func (s *server) RecordCosts(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	list, err := s.o.Records.Costs(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) AddCost(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	input, ok := readBody(w, r)
	if !ok {
		return
	}
	c, err := s.o.Records.AddCost(r.Context(), typeName, id, input, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(c.Version, 10)))
	writeJSON(w, http.StatusCreated, c)
}

func (s *server) ChangeCost(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, costID gen.CostId, p gen.ChangeCostParams) {
	if p.IfMatch == nil || strings.TrimSpace(*p.IfMatch) == "" {
		writeProblem(w, r, http.StatusPreconditionRequired, "precondition_required",
			"send If-Match with the version of the cost entry as you last saw it", nil)
		return
	}
	version, err := parseETag(*p.IfMatch)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "If-Match must be the version of the cost entry, e.g. \"1\"", nil)
		return
	}
	input, ok := readBody(w, r)
	if !ok {
		return
	}
	c, err := s.o.Records.UpdateCost(r.Context(), typeName, id, costID, version, input, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(c.Version, 10)))
	writeJSON(w, http.StatusOK, c)
}

func (s *server) RemoveCost(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, costID gen.CostId) {
	if err := s.o.Records.RemoveCost(r.Context(), typeName, id, costID, actor(r)); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
