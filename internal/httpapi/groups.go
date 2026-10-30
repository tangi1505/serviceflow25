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
	"group24.de/itsm/internal/query"
)

// --- persons and groups (PL7) ---

func (s *server) GroupMembers(w http.ResponseWriter, r *http.Request, id gen.Id, p gen.GroupMembersParams) {
	recursive := p.Recursive != nil && *p.Recursive
	// Only who may read the group learns its members: a group of another tenant is not there
	// (PL44).
	if _, err := s.o.Records.Get(r.Context(), "core.group", id); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	members, err := s.o.Groups.Members(r.Context(), id, recursive)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if members == nil {
		members = []query.Member{}
	}
	writeJSON(w, http.StatusOK, members)
}
