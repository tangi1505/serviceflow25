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

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/org"
)

// --- roles and assignments (PL8) ---
//
// Managing roles is itself a right: the special action "manage_rights" on the type "core.role"
// (granted to admin through * *). Everyone signed in may read the roles.

const rightsType = "core.role"

func (s *server) mayManageRights(w http.ResponseWriter, r *http.Request) bool {
	if a := authz.From(r.Context()); !a.Allowed(rightsType, "manage_rights") {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "managing roles needs the right manage_rights on core.role", nil)
		return false
	}
	return true
}

// mayChangeRoles answers 403 unless the right manage_rights comes from a role at the root:
// roles belong to the installation, a change reaches every tenant (PL44, E136).
func (s *server) mayChangeRoles(w http.ResponseWriter, r *http.Request) bool {
	if a := authz.From(r.Context()); !a.Installation(rightsType, "manage_rights") {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "roles belong to the installation: changing them needs manage_rights on core.role at the root", nil)
		return false
	}
	return true
}

// assignable reports whether an assignment at the node belongs to the session's tenant, or
// to the root for an administrator of the installation (PL44).
func assignable(a *authz.Access, node string) bool {
	return a.InTenant(node) || (node == org.Root && a.Installation(rightsType, "manage_rights"))
}

func (s *server) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.o.Authz.Roles(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if roles == nil {
		roles = []authz.Role{}
	}
	writeJSON(w, http.StatusOK, roles)
}

func (s *server) CreateRole(w http.ResponseWriter, r *http.Request) {
	if !s.mayChangeRoles(w, r) {
		return
	}
	var body gen.RoleCreate
	if !decode(w, r, &body) {
		return
	}
	var label, description map[string]string
	if body.Label != nil {
		label = *body.Label
	}
	if body.Description != nil {
		description = *body.Description
	}
	role, err := s.o.Authz.CreateRole(r.Context(), body.Name, label, description)
	s.changed(r, "role.create", body.Name, nil, role, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/roles/"+role.Name)
	writeJSON(w, http.StatusCreated, role)
}

func (s *server) GetRole(w http.ResponseWriter, r *http.Request, name gen.RoleName) {
	role, err := s.o.Authz.Role(r.Context(), name)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (s *server) DeleteRole(w http.ResponseWriter, r *http.Request, name gen.RoleName) {
	if !s.mayChangeRoles(w, r) {
		return
	}
	before, _ := s.o.Authz.Role(r.Context(), name)
	err := s.o.Authz.DeleteRole(r.Context(), name)
	s.changed(r, "role.delete", name, before, nil, err)
	s.noContent(w, r, err)
}

func (s *server) SetPermission(w http.ResponseWriter, r *http.Request, name gen.RoleName) {
	if !s.mayChangeRoles(w, r) {
		return
	}
	var body gen.Permission
	if !decode(w, r, &body) {
		return
	}
	p := authz.Permission{TypeName: body.TypeName, Action: body.Action, Scope: authz.Scope(body.Scope)}
	if body.Condition != nil {
		p.Condition = *body.Condition
	}
	if body.Fields != nil {
		raw, _ := json.Marshal(body.Fields)
		var f authz.Fields
		if err := json.Unmarshal(raw, &f); err != nil {
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "fields must be {\"read\": [...], \"write\": [...]}", nil)
			return
		}
		p.Fields = &f
	}
	before := s.permissionOf(r, name, p.TypeName, p.Action, p.Scope)
	err := s.o.Authz.SetPermission(r.Context(), name, p)
	s.changed(r, "permission.set", name+" "+p.TypeName+" "+p.Action, before, p, err)
	s.noContent(w, r, err)
}

func (s *server) UnsetPermission(w http.ResponseWriter, r *http.Request, name gen.RoleName, typeName string, action string, p gen.UnsetPermissionParams) {
	if !s.mayChangeRoles(w, r) {
		return
	}
	var scope authz.Scope
	if p.Scope != nil {
		scope = authz.Scope(*p.Scope)
	}
	before := s.permissionOf(r, name, typeName, action, scope)
	err := s.o.Authz.UnsetPermission(r.Context(), name, typeName, action, scope)
	s.changed(r, "permission.unset", name+" "+typeName+" "+action, before, nil, err)
	s.noContent(w, r, err)
}

// permissionOf is the permission of a role for a type, action and scope as it is now, nil
// without one; without a scope the list of every scope (a role may hold several, UI4).
func (s *server) permissionOf(r *http.Request, role, typeName, action string, scope authz.Scope) any {
	got, err := s.o.Authz.Role(r.Context(), role)
	if err != nil {
		return nil
	}
	var out []authz.Permission
	for _, p := range got.Permissions {
		if p.TypeName == typeName && p.Action == action && (scope == "" || p.Scope == scope) {
			out = append(out, p)
		}
	}
	switch {
	case out == nil:
		return nil
	case scope != "":
		return out[0]
	}
	return out
}

// ListAssignments lists the assignments in the session's tenant (PL44).
func (s *server) ListAssignments(w http.ResponseWriter, r *http.Request) {
	list, err := s.o.Authz.Assignments(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	a := authz.From(r.Context())
	mine := []authz.Assignment{}
	for _, x := range list {
		if a.InTenant(x.OrgPath) {
			mine = append(mine, x)
		}
	}
	writeJSON(w, http.StatusOK, mine)
}

func (s *server) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	if !s.mayManageRights(w, r) {
		return
	}
	var body gen.AssignmentCreate
	if !decode(w, r, &body) {
		return
	}
	var person, group *uuid.UUID
	if body.PersonId != nil {
		p := *body.PersonId
		person = &p
	}
	if body.GroupId != nil {
		g := *body.GroupId
		group = &g
	}
	access := authz.From(r.Context())
	node := access.HomeNode()
	if body.OrgPath != nil && *body.OrgPath != "" {
		node = *body.OrgPath
	}
	// In a tenant the node, the person and the group belong to it (PL44); anything else is
	// not there.
	if !assignable(access, node) {
		writeError(w, r, s.o.Logger, fmt.Errorf("%w: node %q", authz.ErrNotFound, node))
		return
	}
	if access.Bounded() {
		holder, typeName := person, "core.person"
		if group != nil {
			holder, typeName = group, "core.group"
		}
		// The person of an account of the installation lives at the root, outside the tenant:
		// an administrator with the rights at the root assigns it from any tenant, checked
		// centrally, in both operating modes (PL48, your decision of 2026-10-31).
		central := false
		if person != nil && access.Installation(rightsType, "manage_rights") {
			ok, err := s.o.Authz.InstallationPerson(r.Context(), *person)
			if err != nil {
				writeError(w, r, s.o.Logger, err)
				return
			}
			central = ok
		}
		if holder != nil && !central {
			if _, err := s.o.Records.Get(r.Context(), typeName, *holder); err != nil {
				writeError(w, r, s.o.Logger, err)
				return
			}
		}
	}
	a, err := s.o.Authz.Assign(r.Context(), body.Role, person, group, node, actor(r))
	s.changed(r, "assignment.create", body.Role, nil, a, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *server) DeleteAssignment(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if !s.mayManageRights(w, r) {
		return
	}
	var before any
	if list, err := s.o.Authz.Assignments(r.Context()); err == nil {
		for _, a := range list {
			if a.ID == id {
				if !assignable(authz.From(r.Context()), a.OrgPath) {
					writeError(w, r, s.o.Logger, authz.ErrNotFound)
					return
				}
				before = a
			}
		}
	}
	err := s.o.Authz.Unassign(r.Context(), id)
	s.changed(r, "assignment.delete", id.String(), before, nil, err)
	s.noContent(w, r, err)
}
