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
	"net/http"

	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
)

// --- LDAP directories (PL12) ---
//
// Managing directories is part of managing rights (manage_rights on core.role): whoever may
// hand out roles may also decide where persons and groups come from.

func (s *server) directoriesReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Authn == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "directories are not configured on this instance", nil)
		return false
	}
	return s.mayManageRights(w, r)
}

func directoryInput(body gen.DirectoryInput) authn.DirectoryInput {
	in := authn.DirectoryInput{URL: body.Url, BindDN: body.BindDn, BaseDN: body.BaseDn, PersonFilter: body.PersonFilter,
		GroupFilter: body.GroupFilter, OrgPath: body.OrgPath, StartTLS: body.StartTls, SkipVerify: body.SkipVerify, Enabled: body.Enabled}
	if body.PageSize != nil {
		p := int32(*body.PageSize) //nolint:gosec // the schema bounds the page size
		in.PageSize = &p
	}
	if body.Attributes != nil {
		a := body.Attributes
		attrs := authn.Attributes{}
		set := func(dst *string, src *string) {
			if src != nil {
				*dst = *src
			}
		}
		set(&attrs.ID, a.Id)
		set(&attrs.Username, a.Username)
		set(&attrs.FirstName, a.FirstName)
		set(&attrs.LastName, a.LastName)
		set(&attrs.Email, a.Email)
		set(&attrs.Phone, a.Phone)
		set(&attrs.Mobile, a.Mobile)
		set(&attrs.GroupName, a.GroupName)
		set(&attrs.GroupMember, a.GroupMember)
		in.Attributes = &attrs
	}
	return in
}

func (s *server) ListDirectories(w http.ResponseWriter, r *http.Request) {
	if !s.directoriesReady(w, r) {
		return
	}
	list, err := s.o.Authn.Directories(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	a := authz.From(r.Context())
	mine := []authn.Directory{}
	for _, d := range list {
		if assignable(a, d.OrgPath) {
			mine = append(mine, d)
		}
	}
	writeJSON(w, http.StatusOK, mine)
}

// directoryNode puts the persons and groups of a directory into the tenant of the session
// unless a node of it is named; a node outside is not there (PL45).
func (s *server) directoryNode(w http.ResponseWriter, r *http.Request, in *authn.DirectoryInput, fill bool) bool {
	a := authz.From(r.Context())
	if in.OrgPath == nil && fill {
		home := a.HomeNode()
		in.OrgPath = &home
	}
	if in.OrgPath != nil && !assignable(a, *in.OrgPath) {
		writeError(w, r, s.o.Logger, fmt.Errorf("%w: node %q", authz.ErrNotFound, *in.OrgPath))
		return false
	}
	return true
}

// directoryInTenant answers 404 for a directory of another tenant.
func (s *server) directoryInTenant(w http.ResponseWriter, r *http.Request, name string) bool {
	d, err := s.o.Authn.Directory(r.Context(), name)
	if err == nil && !assignable(authz.From(r.Context()), d.OrgPath) {
		err = fmt.Errorf("%w: directory %s", authz.ErrNotFound, name)
	}
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return false
	}
	return true
}

func (s *server) CreateDirectory(w http.ResponseWriter, r *http.Request) {
	if !s.directoriesReady(w, r) {
		return
	}
	var body gen.DirectoryCreate
	if !decode(w, r, &body) {
		return
	}
	// The generated DirectoryCreate carries the input's fields flat (allOf); the same JSON
	// read as DirectoryInput gives the settings.
	var in gen.DirectoryInput
	raw, _ := json.Marshal(body)
	_ = json.Unmarshal(raw, &in)
	input := directoryInput(in)
	if !s.directoryNode(w, r, &input, true) {
		return
	}
	d, err := s.o.Authn.CreateDirectory(r.Context(), body.Name, input, actor(r))
	s.changed(r, "directory.create", body.Name, nil, d, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/directories/"+d.Name)
	writeJSON(w, http.StatusCreated, d)
}

func (s *server) GetDirectory(w http.ResponseWriter, r *http.Request, name gen.DirectoryName) {
	if !s.directoriesReady(w, r) || !s.directoryInTenant(w, r, name) {
		return
	}
	d, err := s.o.Authn.Directory(r.Context(), name)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) UpdateDirectory(w http.ResponseWriter, r *http.Request, name gen.DirectoryName) {
	if !s.directoriesReady(w, r) {
		return
	}
	var body gen.DirectoryInput
	if !decode(w, r, &body) {
		return
	}
	input := directoryInput(body)
	if !s.directoryInTenant(w, r, name) || !s.directoryNode(w, r, &input, false) {
		return
	}
	before, _ := s.o.Authn.Directory(r.Context(), name)
	d, err := s.o.Authn.UpdateDirectory(r.Context(), name, input, actor(r))
	s.changed(r, "directory.update", name, before, d, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) DeleteDirectory(w http.ResponseWriter, r *http.Request, name gen.DirectoryName) {
	if !s.directoriesReady(w, r) {
		return
	}
	if !s.directoryInTenant(w, r, name) {
		return
	}
	before, _ := s.o.Authn.Directory(r.Context(), name)
	err := s.o.Authn.DeleteDirectory(r.Context(), name)
	s.changed(r, "directory.delete", name, before, nil, err)
	s.noContent(w, r, err)
}

func (s *server) SyncDirectory(w http.ResponseWriter, r *http.Request, name gen.DirectoryName) {
	if !s.directoriesReady(w, r) || !s.directoryInTenant(w, r, name) {
		return
	}
	job, err := s.o.Authn.EnqueueSync(r.Context(), s.o.Jobs, name)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}
