// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package customer changes types in the customer layer (PL43, Hauptplan E135, E37): what the
// interface changes at a type lands as a list of changes next to the base layer the package
// ships; the effective type is both together (meta.Types). A change is checked against the
// effective type before it is kept, its column is added at once, and a change can be taken
// back as long as no later change needs it. The layers export as the start of a package.
package customer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/schema"
)

// Statuses of a change in a view.
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
	StatusConflict = "conflict"
)

// View is the customer layer of a type as the API shows it.
type View struct {
	Type    string  `json:"type"`
	Version int64   `json:"version"`
	Changes []Entry `json:"changes"`
}

// Entry is one change with whether it applies, and the name of the account that made it
// (UI25).
type Entry struct {
	meta.Change
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	ByName string `json:"by_name,omitempty"`
}

// Service keeps the customer layers.
type Service struct {
	db     *db.Router
	rec    *records.Service
	logger *slog.Logger
}

// New returns the service.
func New(rt *db.Router, rec *records.Service, logger *slog.Logger) *Service {
	return &Service{db: rt, rec: rec, logger: logger}
}

// base returns the type as its package ships it.
func (s *Service) base(ctx context.Context, typeName string) (meta.Type, error) {
	ts, err := meta.BaseTypes(ctx, queries.New(s.db.Control()))
	if err != nil {
		return meta.Type{}, err
	}
	for _, t := range ts {
		if t.QualifiedName() == typeName {
			return t, nil
		}
	}
	return meta.Type{}, fmt.Errorf("type %s: %w", typeName, records.ErrNotFound)
}

// view describes a layer against the base.
func view(t meta.Type, l meta.CustomerLayer, version int64) View {
	_, conflicts := meta.ApplyCustomer(t, l)
	reasons := map[string]string{}
	for _, c := range conflicts {
		reasons[c.Change.ID] = c.Reason
	}
	v := View{Type: t.QualifiedName(), Version: version, Changes: []Entry{}}
	for _, c := range l.Changes {
		e := Entry{Change: c, Status: StatusActive}
		switch {
		case c.Inactive != "":
			e.Status, e.Reason = StatusInactive, c.Inactive
		case reasons[c.ID] != "":
			e.Status, e.Reason = StatusConflict, reasons[c.ID]
		}
		v.Changes = append(v.Changes, e)
	}
	return v
}

// Get returns the customer layer of a type.
func (s *Service) Get(ctx context.Context, typeName string) (View, error) {
	t, err := s.base(ctx, typeName)
	if err != nil {
		return View{}, err
	}
	q := queries.New(s.db.Control())
	l, version, err := meta.GetCustomerLayer(ctx, q, t.Package, t.Name)
	if err != nil {
		return View{}, err
	}
	vs := []View{view(t, l, version)}
	if err := nameAuthors(ctx, q, vs); err != nil {
		return View{}, err
	}
	return vs[0], nil
}

// nameAuthors fills in the display names of the accounts that made the changes, with one
// query; the accounts live in the control database (PL47b).
func nameAuthors(ctx context.Context, q *queries.Queries, vs []View) error {
	var ids []uuid.UUID
	for _, v := range vs {
		for _, e := range v.Changes {
			if id, err := uuid.Parse(e.By); err == nil && id != uuid.Nil && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.AccountNames(ctx, ids)
	if err != nil {
		return fmt.Errorf("customer: names: %w", err)
	}
	names := make(map[string]string, len(rows))
	for _, r := range rows {
		names[r.ID.String()] = r.DisplayName
	}
	// The views share the backing arrays of their changes, so the copies write through.
	for _, v := range vs {
		for k, e := range v.Changes {
			v.Changes[k].ByName = names[e.By]
		}
	}
	return nil
}

// problems turns a refusal into the 422 of the API.
func problems(field string, msgs ...string) error {
	ps := make([]records.Problem, 0, len(msgs))
	for _, m := range msgs {
		ps = append(ps, records.Problem{Field: field, Code: "invalid", Message: m})
	}
	return &records.ValidationError{Problems: ps}
}

// Add checks a change against the effective type, keeps it if the layer still has the
// expected version, and adds its column at once.
func (s *Service) Add(ctx context.Context, typeName string, c meta.Change, expected int64, by uuid.UUID) (View, error) {
	// The types in memory follow the layer at once in this process (PL53).
	defer meta.DefinitionsChanged()
	t, err := s.base(ctx, typeName)
	if err != nil {
		return View{}, err
	}
	if t.IsList() && c.Op == meta.OpAddField {
		return View{}, problems("op", "a select list keeps its entries in a core table and takes no fields of its own")
	}
	// A state with records in it stays (UI26.1), in every database (PL47).
	if c.Op == meta.OpRemoveState {
		for _, d := range s.db.Databases() {
			found, err := s.rec.InState(d.Bind(ctx), typeName, c.Field)
			if err != nil {
				return View{}, err
			}
			if found {
				return View{}, problems(c.Field, fmt.Sprintf("records of %s stand in state %s", typeName, c.Field))
			}
		}
	}
	q := queries.New(s.db.Control())
	l, version, err := meta.GetCustomerLayer(ctx, q, t.Package, t.Name)
	if err != nil {
		return View{}, err
	}
	if version != expected {
		return View{}, fmt.Errorf("customer layer of %s is at version %d, not %d: %w", typeName, version, expected, records.ErrConflict)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return View{}, err
	}
	c.ID, c.At, c.By, c.Inactive = id.String(), time.Now().UTC(), by.String(), ""
	next := meta.CustomerLayer{Changes: append(slices.Clone(l.Changes), c)}
	eff, conflicts := meta.ApplyCustomer(t, next)
	for _, k := range conflicts {
		if k.Change.ID == c.ID {
			return View{}, problems(c.Field, k.Reason)
		}
	}
	known, err := meta.InstalledTypes(ctx, q)
	if err != nil {
		return View{}, err
	}
	bases, err := meta.BaseTypes(ctx, q)
	if err != nil {
		return View{}, err
	}
	traits, err := meta.Traits(ctx, q)
	if err != nil {
		return View{}, err
	}
	if err := meta.ValidateCustomer(eff, bases, traits, known); err != nil {
		if verr, ok := meta.AsValidationError(err); ok {
			var msgs []string
			for _, p := range verr.Problems {
				msgs = append(msgs, p.Error())
			}
			return View{}, problems(c.Field, msgs...)
		}
		return View{}, err
	}
	if _, err := meta.SaveCustomerLayer(ctx, q, t.Package, t.Name, next, version); err != nil {
		if errors.Is(err, meta.ErrLayerChanged) {
			return View{}, fmt.Errorf("customer layer of %s: %w", typeName, records.ErrConflict)
		}
		return View{}, err
	}
	// The new column right away, in every database with type tables (PL47): the type is used
	// with it on the next request.
	if !eff.IsList() {
		for _, pool := range s.db.Pools() {
			if _, err := schema.Apply(ctx, pool, []meta.Type{eff}, s.logger); err != nil {
				return View{}, err
			}
		}
	}
	// A new running number: the records there are get theirs at once, in the order of their
	// creation, and can be found by it (PL49, Kern-Plan E63).
	if c.Op == meta.OpAddField && c.New != nil && c.New.Type == meta.FieldSequence {
		for _, d := range s.db.Databases() {
			bound := d.Bind(ctx)
			if _, err := s.rec.Number(bound, typeName); err != nil {
				return View{}, err
			}
			if _, err := s.rec.RecomputeSearch(bound, typeName); err != nil {
				return View{}, err
			}
		}
	}
	s.logger.Info("customer layer changed", "type", typeName, "op", c.Op, "field", c.Field, "by", by)
	return s.Get(ctx, typeName)
}

// Remove takes a change back if no later change needs it. A field the layer added keeps its
// column and its values; added again under the same name, it shows them again.
func (s *Service) Remove(ctx context.Context, typeName, changeID string, expected int64) (View, error) {
	defer meta.DefinitionsChanged()
	t, err := s.base(ctx, typeName)
	if err != nil {
		return View{}, err
	}
	q := queries.New(s.db.Control())
	l, version, err := meta.GetCustomerLayer(ctx, q, t.Package, t.Name)
	if err != nil {
		return View{}, err
	}
	if version != expected {
		return View{}, fmt.Errorf("customer layer of %s is at version %d, not %d: %w", typeName, version, expected, records.ErrConflict)
	}
	i := slices.IndexFunc(l.Changes, func(c meta.Change) bool { return c.ID == changeID })
	if i < 0 {
		return View{}, fmt.Errorf("change %s of %s: %w", changeID, typeName, records.ErrNotFound)
	}
	_, before := meta.ApplyCustomer(t, l)
	next := meta.CustomerLayer{Changes: slices.Delete(slices.Clone(l.Changes), i, i+1)}
	_, after := meta.ApplyCustomer(t, next)
	was := map[string]bool{}
	for _, c := range before {
		was[c.Change.ID] = true
	}
	var broken []string
	for _, c := range after {
		if !was[c.Change.ID] {
			broken = append(broken, fmt.Sprintf("change %s (%s %s) needs it: %s", c.Change.ID, c.Change.Op, c.Change.Field, c.Reason))
		}
	}
	if len(broken) > 0 {
		return View{}, problems("change", broken...)
	}
	if _, err := meta.SaveCustomerLayer(ctx, q, t.Package, t.Name, next, version); err != nil {
		if errors.Is(err, meta.ErrLayerChanged) {
			return View{}, fmt.Errorf("customer layer of %s: %w", typeName, records.ErrConflict)
		}
		return View{}, err
	}
	s.logger.Info("customer layer change taken back", "type", typeName, "change", changeID)
	return s.Get(ctx, typeName)
}

// Export returns every customer layer, by type name: the content of a package of the
// customer (architektur.md §4); importing it comes with EX.
func (s *Service) Export(ctx context.Context) ([]View, error) {
	q := queries.New(s.db.Control())
	layers, versions, err := meta.CustomerLayers(ctx, q)
	if err != nil {
		return nil, err
	}
	bases, err := meta.BaseTypes(ctx, q)
	if err != nil {
		return nil, err
	}
	out := []View{}
	for _, t := range bases {
		if l, ok := layers[t.QualifiedName()]; ok && len(l.Changes) > 0 {
			out = append(out, view(t, l, versions[t.QualifiedName()]))
		}
	}
	if err := nameAuthors(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateConflicts compares the customer layers with the types of a package about to be
// installed: the changes that would no longer apply, by type. Types the package no longer
// ships count with all their changes.
func UpdateConflicts(ctx context.Context, q *queries.Queries, p meta.Package) (map[string][]meta.Conflict, error) {
	layers, _, err := meta.CustomerLayers(ctx, q)
	if err != nil {
		return nil, err
	}
	out := map[string][]meta.Conflict{}
	for name, l := range layers {
		pkg, typ, _ := strings.Cut(name, ".")
		if pkg != p.Name {
			continue
		}
		t, ok := p.Type(typ)
		if !ok {
			for _, c := range l.Changes {
				if c.Inactive == "" {
					out[name] = append(out[name], meta.Conflict{Change: c, Reason: fmt.Sprintf("the package no longer ships %s", name)})
				}
			}
			continue
		}
		t.Package = p.Name
		if _, cs := meta.ApplyCustomer(t, l); len(cs) > 0 {
			out[name] = cs
		}
	}
	return out, nil
}

// Still marks the conflicting changes inactive with their reason (a forced update).
func Still(ctx context.Context, q *queries.Queries, conflicts map[string][]meta.Conflict) error {
	for name, cs := range conflicts {
		pkg, typ, _ := strings.Cut(name, ".")
		l, version, err := meta.GetCustomerLayer(ctx, q, pkg, typ)
		if err != nil {
			return err
		}
		reason := map[string]string{}
		for _, c := range cs {
			reason[c.Change.ID] = c.Reason
		}
		for i, c := range l.Changes {
			if r, ok := reason[c.ID]; ok {
				l.Changes[i].Inactive = r
			}
		}
		if _, err := meta.SaveCustomerLayer(ctx, q, pkg, typ, l, version); err != nil {
			return err
		}
	}
	return nil
}
