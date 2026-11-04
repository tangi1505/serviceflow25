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

// --- text modules (SD8) ---

// RenderSnippet answers a text module filled in against the record, for the editor.
func (s *server) RenderSnippet(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	if s.o.Snippets == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "the text modules are not configured", nil)
		return
	}
	var body gen.SnippetRequest
	if !decode(w, r, &body) {
		return
	}
	text, err := s.o.Snippets.Render(r.Context(), body.Snippet, typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, gen.SnippetText{Body: text})
}
