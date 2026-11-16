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
	openapi_types "github.com/oapi-codegen/runtime/types"

	"group24.de/itsm/internal/httpapi/gen"
)

// The inbox of the session's person (WF19): only her own entries, whatever the rights.

func (s *server) inboxReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Notify == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "the inbox is not set up", nil)
		return false
	}
	return true
}

func (s *server) MyInbox(w http.ResponseWriter, r *http.Request, p gen.MyInboxParams) {
	if !s.inboxReady(w, r) {
		return
	}
	unread, limit, after := p.Unread != nil && *p.Unread, 0, ""
	if p.Limit != nil {
		limit = *p.Limit
	}
	if p.After != nil {
		after = *p.After
	}
	page, err := s.o.Notify.Inbox(r.Context(), unread, limit, after)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *server) ReadMyInbox(w http.ResponseWriter, r *http.Request) {
	if !s.inboxReady(w, r) {
		return
	}
	var body struct {
		IDs []uuid.UUID `json:"ids"`
		All bool        `json:"all"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.IDs) > 500 {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "at most 500 entries at a time", nil)
		return
	}
	n, err := s.o.Notify.MarkRead(r.Context(), body.IDs, body.All)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"unread": n})
}

func (s *server) RemoveInboxEntry(w http.ResponseWriter, r *http.Request, entryID openapi_types.UUID) {
	if !s.inboxReady(w, r) {
		return
	}
	if err := s.o.Notify.RemoveEntry(r.Context(), entryID); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
