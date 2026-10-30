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

	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/records"
)

// --- batch of changes (UI6, Hauptplan E126) ---

func (s *server) BatchRecords(w http.ResponseWriter, r *http.Request, typeName gen.TypeName) {
	var body struct {
		Rows []records.BatchRow `json:"rows"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "body must be {\"rows\": [...]}: "+err.Error(), nil)
		return
	}
	results, err := s.o.Records.Batch(r.Context(), typeName, body.Rows, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
