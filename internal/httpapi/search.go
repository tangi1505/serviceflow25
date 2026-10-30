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
	"group24.de/itsm/internal/search"
)

// --- full-text search (PL26, Hauptplan E21, E101) ---

func (s *server) Search(w http.ResponseWriter, r *http.Request, p gen.SearchParams) {
	if s.o.Search == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "search is not configured on this instance", nil)
		return
	}
	q := search.Query{Text: p.Q}
	if p.Types != nil && *p.Types != "" {
		q.Types = strings.Split(*p.Types, ",")
	}
	if p.Limit != nil {
		q.Limit = *p.Limit
	}
	hits, err := s.o.Search.Search(r.Context(), q)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, hits)
}
