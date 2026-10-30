// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/richtext"
)

// --- timeline (PL42, Hauptplan E121) ---

func (s *server) RecordTimeline(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	items, err := s.o.Records.Timeline(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) AddTimelineEntry(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	input, ok := readBody(w, r)
	if !ok {
		return
	}
	// Images in formatted text (UI7) are uploads of the caller; they are attached to the
	// record with the entry once it is written.
	var images []uuid.UUID
	if body, _ := input["body"].(string); input["format"] == records.FormatHTML {
		images = richtext.Images(richtext.Sanitize(body))
	}
	if len(images) > 0 {
		t, err := s.o.Records.Type(r.Context(), typeName)
		if err != nil {
			writeError(w, r, s.o.Logger, err)
			return
		}
		if s.o.Files == nil || !t.HasTrait(meta.TraitDocuments) {
			writeError(w, r, s.o.Logger, &records.ValidationError{Problems: []records.Problem{{Field: "body", Code: "images",
				Message: typeName + " takes no images in entries: it needs the trait documents and the file store"}}})
			return
		}
		for _, f := range images {
			if !s.o.Files.Readable(r.Context(), f) {
				writeError(w, r, s.o.Logger, &records.ValidationError{Problems: []records.Problem{{Field: "body", Code: "image",
					Message: "no file " + f.String() + " to show; upload it first (POST /api/v1/files)"}}})
				return
			}
		}
	}
	e, err := s.o.Records.AddEntry(r.Context(), typeName, id, input, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	for _, f := range images {
		if _, err := s.o.Files.AttachEntry(r.Context(), typeName, id, e, f, actor(r)); err != nil {
			// The entry stands; an image that could not be kept shows as missing.
			s.o.Logger.Error("timeline: attach image", "entry", e.ID, "file", f, "error", err.Error())
		}
	}
	writeJSON(w, http.StatusCreated, e)
}
