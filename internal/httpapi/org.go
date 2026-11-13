// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
)

// --- organisation tree (PL6) ---

// ListOrgNodes lists the tree of the session's tenant (PL44): with tenants the tenant node
// and what lies below, without the root and the other tenants.
func (s *server) ListOrgNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.o.Org.Nodes(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	a := authz.From(r.Context())
	mine := []org.Node{}
	for _, n := range nodes {
		if a.InTenant(n.Path) {
			mine = append(mine, n)
		}
	}
	writeJSON(w, http.StatusOK, mine)
}

// inTenant answers 404 for a node outside the session's tenant (PL44): it is not there.
func (s *server) inTenant(w http.ResponseWriter, r *http.Request, path string) bool {
	if !authz.From(r.Context()).InTenant(path) {
		writeError(w, r, s.o.Logger, org.ErrNotFound)
		return false
	}
	return true
}

// orgType is the pseudo type of the rights on the organisation tree: manage_org for its
// nodes, manage_settings for the settings at them. Both reach as far as the assignment does,
// so a unit can manage its subtree; the root and its settings (security among them) need an
// assignment at the root. Reading stays open to every signed-in session.
const orgType = "core.org"

// mayOrg answers 403 unless the session holds the right for the node. The root belongs to
// the installation: with tenants only a role at the root reaches it (PL44); a node of another
// tenant is not there (404).
func (s *server) mayOrg(w http.ResponseWriter, r *http.Request, action, path string) bool {
	a := authz.From(r.Context())
	if a.Permits(orgType, action, map[string]any{"org_path": path}) || (path == org.Root && a.Installation(orgType, action)) {
		return true
	}
	if !s.inTenant(w, r, path) {
		return false
	}
	writeProblem(w, r, http.StatusForbidden, "forbidden", "this needs the right "+action+" on "+orgType+" for "+path, nil)
	return false
}

func (s *server) CreateOrgNode(w http.ResponseWriter, r *http.Request) {
	var body gen.OrgNodeCreate
	if !decode(w, r, &body) {
		return
	}
	// A new node belongs to its parent's subtree.
	if !s.mayOrg(w, r, "manage_org", body.Parent) {
		return
	}
	var label map[string]string
	if body.Label != nil {
		label = *body.Label
	}
	node, err := s.o.Org.Create(r.Context(), body.Parent, body.Name, label)
	s.changed(r, "org_node.create", node.Path, nil, node, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/org/nodes/"+node.Path)
	writeJSON(w, http.StatusCreated, node)
}

func (s *server) GetOrgNode(w http.ResponseWriter, r *http.Request, path gen.OrgPath) {
	if !s.inTenant(w, r, path) {
		return
	}
	node, err := s.o.Org.Node(r.Context(), path)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (s *server) DeleteOrgNode(w http.ResponseWriter, r *http.Request, path gen.OrgPath) {
	if !s.mayOrg(w, r, "manage_org", path) {
		return
	}
	before, _ := s.o.Org.Node(r.Context(), path)
	err := s.o.Org.Delete(r.Context(), path)
	s.changed(r, "org_node.delete", path, before, nil, err)
	s.noContent(w, r, err)
}

func (s *server) OrgSettings(w http.ResponseWriter, r *http.Request, path gen.OrgPath) {
	if path != org.Root && !s.inTenant(w, r, path) {
		return
	}
	settings, err := s.o.Org.Settings(r.Context(), path)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if settings == nil {
		settings = []org.Setting{}
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *server) SetOrgSetting(w http.ResponseWriter, r *http.Request, path gen.OrgPath, key gen.SettingKey) {
	if !s.mayOrg(w, r, "manage_settings", path) {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil || !json.Valid(raw) {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "body must be a JSON value", nil)
		return
	}
	before := s.ownSetting(r, path, key)
	if s.o.Settings != nil {
		err = s.o.Settings.Set(r.Context(), path, key, json.RawMessage(raw), actor(r))
	} else {
		err = s.o.Org.SetSetting(r.Context(), path, key, json.RawMessage(raw), actor(r))
	}
	s.changed(r, "setting.set", path+" "+key, before, json.RawMessage(raw), err)
	if err == nil {
		s.o.Auth.ForgetPolicy() // security settings apply at once on this instance (PL28)
	}
	s.noContent(w, r, err)
}

func (s *server) UnsetOrgSetting(w http.ResponseWriter, r *http.Request, path gen.OrgPath, key gen.SettingKey) {
	if !s.mayOrg(w, r, "manage_settings", path) {
		return
	}
	before := s.ownSetting(r, path, key)
	err := s.o.Org.UnsetSetting(r.Context(), path, key)
	s.changed(r, "setting.unset", path+" "+key, before, nil, err)
	if err == nil {
		s.o.Auth.ForgetPolicy()
	}
	s.noContent(w, r, err)
}

// ApplyOrgSettings sets and removes values of a node at once (SD20): all are checked first,
// then written in one transaction; each change gets its own row in the security log.
func (s *server) ApplyOrgSettings(w http.ResponseWriter, r *http.Request, path gen.OrgPath) {
	if !s.mayOrg(w, r, "manage_settings", path) {
		return
	}
	if s.o.Settings == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "settings are not configured", nil)
		return
	}
	var changes map[string]json.RawMessage
	if !decode(w, r, &changes) {
		return
	}
	if len(changes) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	before := map[string]any{}
	for key := range changes {
		before[key] = s.ownSetting(r, path, key)
	}
	err := s.o.Settings.Apply(r.Context(), path, changes, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	for key, value := range changes {
		var after any = value
		action := "setting.set"
		if len(value) == 0 || string(value) == "null" {
			after, action = nil, "setting.unset"
		}
		s.changed(r, action, path+" "+key, before[key], after, nil)
	}
	s.o.Auth.ForgetPolicy()
	w.WriteHeader(http.StatusNoContent)
}

// ListSettingDefinitions gives the declared settings for the page (SD20).
func (s *server) ListSettingDefinitions(w http.ResponseWriter, r *http.Request) {
	if s.o.Settings == nil {
		writeJSON(w, http.StatusOK, []meta.Setting{})
		return
	}
	defs, err := s.o.Settings.Definitions(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if defs == nil {
		defs = []meta.Setting{}
	}
	writeJSON(w, http.StatusOK, defs)
}

// ownSetting is the value a node sets itself (not inherited), nil without one.
func (s *server) ownSetting(r *http.Request, path, key string) any {
	all, err := s.o.Org.Settings(r.Context(), path)
	if err != nil {
		return nil
	}
	for _, st := range all {
		if st.Key == key && st.Source == path {
			return st.Value
		}
	}
	return nil
}
