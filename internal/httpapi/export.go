// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"group24.de/itsm/internal/export"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/seclog"
)

// --- export of a list (UI5, Hauptplan E125) ---

func (s *server) ExportRecords(w http.ResponseWriter, r *http.Request, typeName gen.TypeName) {
	if s.o.Export == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "exports are not configured on this instance", nil)
		return
	}
	var req export.Request
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	if err := dec.Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "body must be an export request: "+err.Error(), nil)
		return
	}
	if req.Lang == "" {
		req.Lang = firstLanguage(r.Header.Get("Accept-Language"))
	}
	ctx := r.Context()
	rows, err := s.o.Export.Count(ctx, typeName, req)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	details := map[string]any{"format": req.Format, "rows": rows, "columns": req.Columns, "sort": req.Sort}
	if len(req.Where) > 0 {
		details["where"] = req.Where
	}
	if rows > s.o.Export.MaxDirect {
		a := actor(r)
		if !s.o.Export.CanQueue() || a == nil {
			writeProblem(w, r, http.StatusServiceUnavailable, "unavailable",
				fmt.Sprintf("%d rows are more than %d; a larger export runs as a job and needs the file store (ITSM_FILES_DIR) and a signed-in caller", rows, s.o.Export.MaxDirect), nil)
			return
		}
		job, err := s.o.Export.Start(ctx, typeName, req, *a)
		if err != nil {
			writeError(w, r, s.o.Logger, err)
			return
		}
		details["job"] = job.ID
		s.secRecord(r, seclog.Event{Category: seclog.Data, Kind: "export", Outcome: seclog.Success, Target: typeName, Details: details})
		w.Header().Set("Location", fmt.Sprintf("/api/v1/jobs/%d", job.ID))
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	// Logged before the first byte: a file that was sent may be incomplete, never unlogged.
	s.secRecord(r, seclog.Event{Category: seclog.Data, Kind: "export", Outcome: seclog.Success, Target: typeName, Details: details})
	name := export.FileName(typeName, req.Format, time.Now())
	w.Header().Set("Content-Type", export.ContentType(req.Format))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("ITSM-Rows", strconv.FormatInt(rows, 10))
	w.Header().Set("Cache-Control", "no-store")
	if _, err := s.o.Export.Write(ctx, w, typeName, req); err != nil {
		// The status is sent; the broken file is all the caller can get.
		s.o.Logger.Error("export broke off", "type", typeName, "format", req.Format, "error", err.Error())
	}
}

// firstLanguage is the first language of an Accept-Language header, "de" without one.
func firstLanguage(h string) string {
	tag, _, _ := strings.Cut(h, ",")
	tag, _, _ = strings.Cut(strings.TrimSpace(tag), ";")
	tag, _, _ = strings.Cut(tag, "-")
	if tag = strings.ToLower(strings.TrimSpace(tag)); tag == "" || tag == "*" {
		return "de"
	}
	return tag
}
