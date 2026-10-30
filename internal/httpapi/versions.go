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
)

// --- versions (PL19, KF17, Hauptplan E96) ---

func (s *server) RecordVersions(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	list, err := s.o.Records.Versions(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) RecordVersion(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, n gen.VersionNo) {
	rec, err := s.o.Records.Version(r.Context(), typeName, id, n)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *server) CompareVersions(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, p gen.CompareVersionsParams) {
	cmp, err := s.o.Records.Compare(r.Context(), typeName, id, p.From, p.To)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, cmp)
}

func (s *server) RestoreVersion(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, n gen.VersionNo, p gen.RestoreVersionParams) {
	if p.IfMatch == nil || strings.TrimSpace(*p.IfMatch) == "" {
		writeProblem(w, r, http.StatusPreconditionRequired, "precondition_required",
			"send If-Match with the ETag of the record as you last saw it", nil)
		return
	}
	version, err := parseETag(*p.IfMatch)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "If-Match must be the ETag of the record, e.g. \"3\"", nil)
		return
	}
	rec, err := s.o.Records.RestoreVersion(r.Context(), typeName, id, n, version, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if !s.localize(w, r, typeName, rec) {
		return
	}
	w.Header().Set("ETag", etag(rec))
	writeJSON(w, http.StatusOK, rec)
}
