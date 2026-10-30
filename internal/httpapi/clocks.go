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

// --- the SLA clocks of a record (WF9) ---
//
// Whoever reads the record reads its clocks; an invisible record is 404. The SLAs themselves
// are records of core.sla.

func (s *server) RecordClocks(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	if s.o.SLA == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "SLA clocks are not configured on this instance", nil)
		return
	}
	list, err := s.o.SLA.Of(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
