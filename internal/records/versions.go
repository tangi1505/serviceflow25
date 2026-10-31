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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// --- versions of a record (KF17; Hauptplan E96; PL19) ---
//
// A version is the state of a record after a write with that version number. It is not
// stored: the history (trait history) holds every field change with old and new value, so
// a version is the current record with every change after it undone, newest first. Rows of
// relations and documents (Trace) carry no field of the record and are skipped. The roles at
// the record (WF3) are part of a version: their rows are "actors.<role>". Restoring writes the
// field values and roles of an old version as a new version through Update.

// VersionInfo is one version in the list.
type VersionInfo struct {
	Version int64      `json:"version"`
	At      time.Time  `json:"at"`
	Actor   *uuid.UUID `json:"actor,omitempty"`
	// Actions are the audit actions of the version in order (create, update, transition, …).
	Actions []string `json:"actions"`
	// Changed are the fields the version changed, in order of the type.
	Changed []string `json:"changed"`
}

// Change is one field that differs between two versions.
type Change struct {
	Field string `json:"field"`
	From  any    `json:"from"`
	To    any    `json:"to"`
}

// Comparison is the difference between two versions of a record.
type Comparison struct {
	From    int64    `json:"from"`
	To      int64    `json:"to"`
	Changes []Change `json:"changes"`
}

// traced are the audit fields that belong to Trace, not to the row of the record.
var traced = map[string]bool{"relation": true, "documents": true, meta.TraitCosts: true, "notify": true, "clocks": true}

// stateField and orgPathField are the system fields a version carries next to the fields.
const stateField = "state"

// versioned is a readable record of a type with history, with its audit rows.
func (s *Service) versioned(ctx context.Context, typeName string, id uuid.UUID) (meta.Type, Record, []queries.Audit, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return meta.Type{}, nil, nil, err
	}
	if !t.HasHistory() {
		return meta.Type{}, nil, nil, fmt.Errorf("%s has no versions (trait history): %w", typeName, ErrNotFound)
	}
	rec, err := s.read(ctx, s.db.Tenant(ctx), t, id, false, false)
	if err != nil {
		return meta.Type{}, nil, nil, err
	}
	if err := checkAccess(ctx, t, rec, authz.ActionRead); err != nil {
		return meta.Type{}, nil, nil, err
	}
	rows, err := queries.New(s.db.Tenant(ctx)).ListAudit(ctx, queries.ListAuditParams{TypeName: typeName, RecordID: id})
	if err != nil {
		return meta.Type{}, nil, nil, fmt.Errorf("records: versions: %w", err)
	}
	return t, rec, rows, nil
}

// Versions lists the versions of a record, oldest first.
func (s *Service) Versions(ctx context.Context, typeName string, id uuid.UUID) ([]VersionInfo, error) {
	t, _, rows, err := s.versioned(ctx, typeName, id)
	if err != nil {
		return nil, err
	}
	readable := readableSet(ctx, t)
	var out []VersionInfo
	byVersion := map[int64]int{}
	for _, a := range rows {
		i, ok := byVersion[a.Version]
		if !ok {
			v := VersionInfo{Version: a.Version, At: a.At.Time, Actions: []string{}, Changed: []string{}}
			if a.Actor.Valid {
				u := a.Actor.UUID
				v.Actor = &u
			}
			out = append(out, v)
			i = len(out) - 1
			byVersion[a.Version] = i
		}
		v := &out[i]
		if len(v.Actions) == 0 || v.Actions[len(v.Actions)-1] != a.Action {
			v.Actions = append(v.Actions, a.Action)
		}
		if a.Field.Valid && !traced[a.Field.String] && readable(a.Field.String) && !contains(v.Changed, a.Field.String) {
			v.Changed = append(v.Changed, a.Field.String)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	if out == nil {
		out = []VersionInfo{}
	}
	return out, nil
}

// Version returns the record as it was after version n.
func (s *Service) Version(ctx context.Context, typeName string, id uuid.UUID, n int64) (Record, error) {
	t, rec, rows, err := s.versioned(ctx, typeName, id)
	if err != nil {
		return nil, err
	}
	v, err := stateAt(t, rec, rows, n)
	if err != nil {
		return nil, err
	}
	return hideFields(ctx, t, v), nil
}

// Compare returns the fields that differ between versions from and to.
func (s *Service) Compare(ctx context.Context, typeName string, id uuid.UUID, from, to int64) (Comparison, error) {
	t, rec, rows, err := s.versioned(ctx, typeName, id)
	if err != nil {
		return Comparison{}, err
	}
	a, err := stateAt(t, rec, rows, from)
	if err != nil {
		return Comparison{}, err
	}
	b, err := stateAt(t, rec, rows, to)
	if err != nil {
		return Comparison{}, err
	}
	a, b = hideFields(ctx, t, a), hideFields(ctx, t, b)
	if hasActors(t) {
		_, aRoles := a[actorsField]
		_, bRoles := b[actorsField]
		if aRoles {
			flatActors(t, a)
		}
		if bRoles {
			flatActors(t, b)
		}
	}
	cmp := Comparison{From: from, To: to, Changes: []Change{}}
	for _, name := range versionFields(t) {
		va, okA := a[name]
		vb, okB := b[name]
		if !okA && !okB {
			continue
		}
		if !equalJSON(va, vb) {
			cmp.Changes = append(cmp.Changes, Change{Field: name, From: va, To: vb})
		}
	}
	return cmp, nil
}

// RestoreVersion writes the field values of version n as a new version. Status and organisation
// node stay as they are, and so do fields the session may not write. expectedVersion is the
// current version the caller saw (If-Match).
func (s *Service) RestoreVersion(ctx context.Context, typeName string, id uuid.UUID, n, expectedVersion int64, actor Actor) (Record, error) {
	t, rec, rows, err := s.versioned(ctx, typeName, id)
	if err != nil {
		return nil, err
	}
	old, err := stateAt(t, rec, rows, n)
	if err != nil {
		return nil, err
	}
	all, writable := authz.From(ctx).WritableFields(t.QualifiedName(), authz.ActionUpdate)
	patch := map[string]any{}
	for _, f := range t.Fields {
		if f.Calculated() || (!all && !writable[f.Name]) {
			continue
		}
		if !equalJSON(rec[f.Name], old[f.Name]) {
			patch[f.Name] = old[f.Name]
		}
	}
	if hasActors(t) && (all || writable[actorsField]) {
		now, _ := rec[actorsField].(map[string]any)
		then, _ := old[actorsField].(map[string]any)
		roles := map[string]any{}
		for _, r := range t.Actors {
			if !equalJSON(now[r.Name], then[r.Name]) {
				roles[r.Name] = then[r.Name]
			}
		}
		if len(roles) > 0 {
			patch[actorsField] = roles
		}
	}
	return s.Update(ctx, typeName, id, expectedVersion, patch, actor)
}

// versionFields are the fields a version compares: the type's fields, state, org_path and
// the roles at the record as "actors.<role>". Computed fields (the sums of the costs, PL20)
// change without a version and are left out.
func versionFields(t meta.Type) []string {
	out := make([]string, 0, len(t.Fields)+2+len(t.Actors))
	for _, f := range t.Fields {
		if !f.Computed {
			out = append(out, f.Name)
		}
	}
	out = append(out, stateField, orgPathField)
	if hasActors(t) {
		for _, r := range t.Actors {
			out = append(out, actorsField+"."+r.Name)
		}
	}
	return out
}

// flatActors puts the roles of a record beside its fields ("actors.assigned"), the way the
// history names them; nestActors folds them back.
func flatActors(t meta.Type, rec Record) {
	roles, _ := rec[actorsField].(map[string]any)
	for _, r := range t.Actors {
		rec[actorsField+"."+r.Name] = roles[r.Name]
	}
	delete(rec, actorsField)
}

func nestActors(t meta.Type, rec Record) {
	if !hasActors(t) {
		return
	}
	roles := make(map[string]any, len(t.Actors))
	for _, r := range t.Actors {
		key := actorsField + "." + r.Name
		roles[r.Name] = rec[key]
		delete(rec, key)
	}
	rec[actorsField] = roles
}

// stateAt undoes every field change after version n, newest first, starting from the record
// as it is now.
func stateAt(t meta.Type, rec Record, rows []queries.Audit, n int64) (Record, error) {
	current, _ := rec["version"].(int64)
	if n < 1 || n > current {
		return nil, fmt.Errorf("version %d of %s %v: %w (versions run from 1 to %d)", n, t.QualifiedName(), rec["id"], ErrNotFound, current)
	}
	// The history must reach back to n: a create row, or rows at or before n (PL39 may
	// remove old rows).
	oldest := int64(0)
	if len(rows) > 0 {
		oldest = rows[0].Version
		for _, a := range rows {
			oldest = min(oldest, a.Version)
		}
	}
	if len(rows) == 0 || oldest > n {
		return nil, fmt.Errorf("version %d of %s %v is no longer in the history: %w", n, t.QualifiedName(), rec["id"], ErrNotFound)
	}
	fields := map[string]bool{}
	for _, name := range versionFields(t) {
		fields[name] = true
	}
	out := Record{"id": rec["id"], "version": n}
	now := Record{}
	for k, v := range rec {
		now[k] = v
	}
	if hasActors(t) {
		flatActors(t, now)
	}
	for _, name := range versionFields(t) {
		out[name] = now[name]
	}
	for i := len(rows) - 1; i >= 0; i-- {
		a := rows[i]
		if a.Version <= n {
			break
		}
		if !a.Field.Valid || !fields[a.Field.String] {
			continue
		}
		var old any
		if len(a.OldValue) > 0 {
			if err := json.Unmarshal(a.OldValue, &old); err != nil {
				return nil, fmt.Errorf("records: version: old value of %s: %w", a.Field.String, err)
			}
		}
		out[a.Field.String] = old
	}
	// The rows are ordered by time; a version always comes after the ones before it, so the
	// loop above may stop at the first row at or below n.
	nestActors(t, out)
	return out, nil
}

// readableSet tells which fields the session may read (system fields always).
func readableSet(ctx context.Context, t meta.Type) func(string) bool {
	all, allowed := authz.From(ctx).ReadableFields(t.QualifiedName())
	return func(name string) bool {
		if all {
			return true
		}
		// A translation ("title.en") is as readable as its field (PL21), a role
		// ("actors.assigned") as the roles (WF3).
		if f, _, ok := strings.Cut(name, "."); ok {
			name = f
		}
		if name == actorsField {
			return allowed[name]
		}
		if _, isField := t.Field(name); !isField {
			return true
		}
		return allowed[name]
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
