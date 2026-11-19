// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
)

// ErrForbidden says the record is visible but the action is not granted (403). A record the
// access may not read answers as ErrNotFound, so that nothing leaks (PL8).
var ErrForbidden = errors.New("forbidden")

// checkAccess applies the rights of the request to one record: not readable → not found,
// readable but the action not granted → forbidden.
func checkAccess(ctx context.Context, t meta.Type, rec Record, action string) error {
	a := authz.From(ctx)
	if a.IsSystem() {
		return nil
	}
	if !a.Permits(t.QualifiedName(), authz.ActionRead, rec) {
		return fmt.Errorf("%s %v: %w", t.QualifiedName(), rec["id"], ErrNotFound)
	}
	if action != authz.ActionRead && !a.Permits(t.QualifiedName(), action, rec) {
		return fmt.Errorf("%w: %s on %s", ErrForbidden, action, t.QualifiedName())
	}
	return nil
}

// checkFields refuses values for fields the access may not write (PL9). The roles at the
// record count as one field, "actors" (WF3).
func checkFields(ctx context.Context, t meta.Type, action string, values map[string]any) error {
	a := authz.From(ctx)
	if a.IsSystem() {
		return nil
	}
	all, allowed := a.WritableFields(t.QualifiedName(), action)
	if all {
		return nil
	}
	var ps []Problem
	for name := range values {
		if _, ok := t.Field(name); (ok || name == actorsField) && !allowed[name] {
			ps = append(ps, Problem{Field: name, Code: "forbidden", Message: "no right to write this field"})
		}
	}
	if len(ps) > 0 {
		sort.Slice(ps, func(i, j int) bool { return ps[i].Field < ps[j].Field })
		return fmt.Errorf("%w: fields %s", ErrForbidden, ps[0].Field)
	}
	return nil
}

// hideFields drops the fields the access may not read (PL9); system fields stay. The roles
// at the record are the field "actors".
func hideFields(ctx context.Context, t meta.Type, rec Record) Record {
	a := authz.From(ctx)
	if a.IsSystem() {
		return rec
	}
	all, allowed := a.ReadableFields(t.QualifiedName())
	if all {
		return rec
	}
	for _, f := range t.Fields {
		if !allowed[f.Name] {
			delete(rec, f.Name)
		}
	}
	if !allowed[actorsField] {
		delete(rec, actorsField)
	}
	return rec
}

// VisibleEvent tells whether the session may see an event (PL24, E87): read on core.event
// sees everything of the tenant (PL45); otherwise the type must be readable and the record as
// it is now (also in the trash); after a purge the org_path and creator carried in the payload
// decide.
func (s *Service) VisibleEvent(ctx context.Context, e Event) bool {
	if seesAll(authz.From(ctx), e) {
		return true
	}
	visible, _ := s.SeeEvent(ctx, e)
	return visible
}

// SeeEvent says whether the session may see the event and whether it concerns a record where
// its person or one of their groups holds a role (UI8). The record is read once for both.
func (s *Service) SeeEvent(ctx context.Context, e Event) (visible, mine bool) {
	a := authz.From(ctx)
	all := seesAll(a, e)
	if !all && !a.Allowed(e.TypeName, authz.ActionRead) {
		return false, false
	}
	t, err := s.Type(ctx, e.TypeName)
	if err != nil {
		return all, false
	}
	if rec, err := s.read(ctx, s.db.Tenant(ctx), t, e.RecordID, true, false); err == nil {
		mine = a.Involved(rec)
		return all || a.Permits(e.TypeName, authz.ActionRead, rec), mine
	}
	if all {
		return true, false
	}
	var probe struct {
		OrgPath   string `json:"org_path"`
		CreatedBy string `json:"created_by"`
	}
	_ = json.Unmarshal(e.Payload, &probe)
	return a.Permits(e.TypeName, authz.ActionRead, map[string]any{"org_path": probe.OrgPath, "created_by": probe.CreatedBy}), false
}

// seesAll reports whether the access sees the event without asking the record: the system,
// and read on core.event for an event of the tenant (PL45).
func seesAll(a *authz.Access, e Event) bool {
	return a.IsSystem() || (a.Allowed("core.event", authz.ActionRead) && a.InTenant(EventPath(e)))
}

// EventPath is the node of the record when the event happened, as its payload carries it.
func EventPath(e Event) string {
	var probe struct {
		OrgPath string `json:"org_path"`
	}
	_ = json.Unmarshal(e.Payload, &probe)
	return probe.OrgPath
}

// VisibleType is the type definition as the access sees it: without the fields it may not
// read (for the type listing and the OpenAPI, PL9).
func VisibleType(ctx context.Context, t meta.Type) meta.Type {
	a := authz.From(ctx)
	t.May = a.Actions(t.QualifiedName())
	if a.IsSystem() {
		return t
	}
	all, allowed := a.ReadableFields(t.QualifiedName())
	if all {
		return t
	}
	out := t
	// An empty list, never nil: the interface iterates over it (JSON null broke the list).
	out.Fields = make([]meta.Field, 0, len(t.Fields))
	for _, f := range t.Fields {
		if allowed[f.Name] {
			out.Fields = append(out.Fields, f)
		}
	}
	if !allowed[actorsField] {
		out.Actors = nil
	}
	return out
}

// createNode is the node a new record lands in: the one it names, else the default of the
// access (PL46: context, person, the one node with the right). When that leaves a choice the
// caller has to name one, never the tenant by stealth (E136).
func createNode(ctx context.Context, t meta.Type, values map[string]any) (string, error) {
	if p, ok := values[orgPathField].(string); ok {
		return p, nil
	}
	node, ok := authz.From(ctx).CreateNode(t.QualifiedName())
	if !ok {
		return "", &ValidationError{Problems: []Problem{{Field: orgPathField, Code: "required",
			Message: "you may create in several nodes; name one"}}}
	}
	return node, nil
}

// syncNode is the node whose tenant the search for an external key stays in (PL46): the one
// the row names, else the home of the access.
func syncNode(ctx context.Context, fields map[string]any) string {
	if p, ok := fields[orgPathField].(string); ok && p != "" {
		return p
	}
	return authz.From(ctx).HomeNode()
}

// checkCreate applies the rights to a record about to be created at a node.
func checkCreate(ctx context.Context, t meta.Type, path string, values map[string]any) error {
	a := authz.From(ctx)
	if a.IsSystem() {
		return nil
	}
	probe := map[string]any{"org_path": path, "created_by": a.AccountID.String()}
	for k, v := range values {
		probe[k] = v
	}
	if !a.Permits(t.QualifiedName(), authz.ActionCreate, probe) {
		return fmt.Errorf("%w: create %s at %s", ErrForbidden, t.QualifiedName(), path)
	}
	return nil
}
