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

	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/suggest"
)

// --- suggestions and ratings (SD18, SD-Plan N13, N14) ---

func (s *server) SuggestRecords(w http.ResponseWriter, r *http.Request, typeName gen.TypeName) {
	if s.o.Relations == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "relations are not configured on this instance", nil)
		return
	}
	var body struct {
		Values map[string]any `json:"values"`
		Text   string         `json:"text"`
		ID     *uuid.UUID     `json:"id"`
		Limit  int            `json:"limit"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len([]rune(body.Text)) > 200 {
		writeError(w, r, s.o.Logger, &records.ValidationError{Problems: []records.Problem{{Field: "text", Code: "max_length", Message: "must have at most 200 characters"}}})
		return
	}
	if body.Limit < 0 || body.Limit > 20 {
		writeError(w, r, s.o.Logger, &records.ValidationError{Problems: []records.Problem{{Field: "limit", Code: "range", Message: "must be between 1 and 20"}}})
		return
	}
	groups, err := suggest.New(s.o.Records, s.o.Relations).Suggest(r.Context(), typeName, suggest.Request{Values: body.Values, Text: body.Text, ID: body.ID, Limit: body.Limit})
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *server) RecordRating(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	sums, err := s.o.Records.Rating(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, sums)
}

func (s *server) RateRecord(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	var body struct {
		Helpful json.RawMessage `json:"helpful"`
	}
	if !decode(w, r, &body) {
		return
	}
	var helpful *bool
	if len(body.Helpful) == 0 || json.Unmarshal(body.Helpful, &helpful) != nil {
		writeError(w, r, s.o.Logger, &records.ValidationError{Problems: []records.Problem{{Field: "helpful", Code: "required", Message: "must be true, false or null"}}})
		return
	}
	sums, err := s.o.Records.Rate(r.Context(), typeName, id, helpful)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, sums)
}
