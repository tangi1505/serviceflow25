// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"errors"
	"mime"
	"net/http"

	"group24.de/itsm/internal/files"
	"group24.de/itsm/internal/httpapi/gen"
)

// --- files (PL18, Hauptplan E22, E95) ---

func (s *server) filesReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Files == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "files are not configured on this instance (ITSM_FILES_DIR)", nil)
		return false
	}
	return true
}

func (s *server) fileError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, files.ErrTooLarge):
		writeProblem(w, r, http.StatusRequestEntityTooLarge, "too_large", err.Error(), nil)
	case errors.Is(err, files.ErrNoTrait):
		writeProblem(w, r, http.StatusNotFound, "not_found", err.Error(), nil)
	default:
		writeError(w, r, s.o.Logger, err)
	}
}

// UploadFile takes the raw body as a new temporary file. The name comes from the query
// parameter name or from Content-Disposition.
func (s *server) UploadFile(w http.ResponseWriter, r *http.Request, p gen.UploadFileParams) {
	if !s.filesReady(w, r) {
		return
	}
	name := ""
	if p.Name != nil {
		name = *p.Name
	}
	if name == "" {
		if _, params, err := mime.ParseMediaType(r.Header.Get("Content-Disposition")); err == nil {
			name = params["filename"]
		}
	}
	f, err := s.o.Files.Upload(r.Context(), name, r.Body, actor(r))
	if err != nil {
		s.fileError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/files/"+f.ID.String())
	writeJSON(w, http.StatusCreated, f)
}

// DownloadFile streams a file as a download (ranges work through http.ServeContent).
func (s *server) DownloadFile(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if !s.filesReady(w, r) {
		return
	}
	f, content, err := s.o.Files.Open(r.Context(), id)
	if err != nil {
		s.fileError(w, r, err)
		return
	}
	defer func() { _ = content.Close() }()
	h := w.Header()
	h.Set("Content-Type", f.ContentType)
	h.Set("Content-Disposition", files.ContentDisposition(f.Name))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("Cache-Control", "private, no-cache")
	h.Set("ETag", `"`+f.SHA256+`"`)
	h.Set("ITSM-SHA256", f.SHA256)
	http.ServeContent(w, r, "", f.CreatedAt, content)
}

// RecordDocuments lists the files attached to a record (trait documents).
func (s *server) RecordDocuments(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	if !s.filesReady(w, r) {
		return
	}
	list, err := s.o.Files.List(r.Context(), typeName, id)
	if err != nil {
		s.fileError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// AttachDocument attaches an uploaded file to a record.
func (s *server) AttachDocument(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	if !s.filesReady(w, r) {
		return
	}
	var body gen.AttachmentCreate
	if !decode(w, r, &body) {
		return
	}
	f, err := s.o.Files.Attach(r.Context(), typeName, id, body.File, actor(r))
	if err != nil {
		s.fileError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

// DetachDocument removes a file from a record.
func (s *server) DetachDocument(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, fileID gen.FileId) {
	if !s.filesReady(w, r) {
		return
	}
	if err := s.o.Files.Detach(r.Context(), typeName, id, fileID, actor(r)); err != nil {
		s.fileError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
