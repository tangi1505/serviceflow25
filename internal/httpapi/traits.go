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
	"group24.de/itsm/internal/meta"
)

// --- traits (PL17, architektur.md B2) ---

// ListTraits answers the core traits and the installed traits of the packages.
func (s *server) ListTraits(w http.ResponseWriter, r *http.Request) {
	traits, err := s.o.Records.Traits(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, traits)
}

// subResource answers a sub-resource a trait opens: 404 when the type lacks the trait, else
// 501 until the task that fills it is built (documents PL18, notes PL35, reservations later).
func (s *server) subResource(w http.ResponseWriter, r *http.Request, typeName, trait, task string) {
	t, err := s.o.Records.Type(r.Context(), typeName)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if !t.HasTrait(trait) {
		writeProblem(w, r, http.StatusNotFound, "not_found", typeName+" has no trait "+trait, nil)
		return
	}
	writeProblem(w, r, http.StatusNotImplemented, "not_implemented", "the trait "+trait+" is declared; its content arrives with "+task, nil)
}

func (s *server) RecordNotes(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, _ gen.Id) {
	s.subResource(w, r, typeName, meta.TraitNotes, "PL35")
}

func (s *server) RecordReservations(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, _ gen.Id) {
	s.subResource(w, r, typeName, meta.TraitReservable, "the reservation package")
}
