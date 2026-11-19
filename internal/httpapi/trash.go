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
	"group24.de/itsm/internal/records"
)

// --- the trash over all types (PL74, Kern-Plan E78) ---

func (s *server) ListTrash(w http.ResponseWriter, r *http.Request, p gen.ListTrashParams) {
	var o records.TrashOptions
	if p.Type != nil {
		o.Type = *p.Type
	}
	if p.Q != nil {
		o.Query = *p.Q
	}
	if p.Limit != nil {
		o.Limit = *p.Limit
	}
	if p.After != nil {
		o.After = *p.After
	}
	page, err := s.o.Records.ListTrash(r.Context(), o)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
