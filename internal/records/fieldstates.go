// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/rules"
)

// --- field states per state and form variant (WF2; architektur.md B5; Inventar G53) ---
//
// The rules of a type (meta.FieldStateRule) say per state, and per variant through a CEL
// condition, which fields are required, hidden, read-only or preset. The server keeps them on
// every write: a new record gets the presets of its initial state and must not set read-only
// fields; a change must not touch fields that are read-only in the state the record is in
// (read_only "*" locks the whole record, roles included); and the state the record ends up in
// (the same, or the target of a transition) must find its required fields filled. Hidden is
// left to the form. FieldStates tells the form what applies to a record or a draft.

// FieldStates is what the rules say about one record now.
type FieldStates struct {
	State    string         `json:"state"`
	Required []string       `json:"required"`
	Hidden   []string       `json:"hidden"`
	ReadOnly []string       `json:"read_only"`
	Locked   bool           `json:"locked"`
	Defaults map[string]any `json:"defaults"`
}

// conditions caches the compiled conditions of the rules by their text.
var conditions sync.Map

func condition(expr string) (*rules.Condition, error) {
	if c, ok := conditions.Load(expr); ok {
		return c.(*rules.Condition), nil
	}
	c, err := rules.Compile(expr)
	if err != nil {
		return nil, err
	}
	conditions.Store(expr, c)
	return c, nil
}

// effective adds up the rules that apply to a record (in its API shape) in a state.
func effective(t meta.Type, state string, rec map[string]any) (FieldStates, error) {
	fs := FieldStates{State: state, Required: []string{}, Hidden: []string{}, ReadOnly: []string{}, Defaults: map[string]any{}}
	required, hidden, readOnly := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, r := range t.FieldStates {
		if !r.AppliesIn(state) {
			continue
		}
		if r.When != "" {
			c, err := condition(r.When)
			if err != nil {
				return fs, err
			}
			ok, err := c.Eval(rec)
			if err != nil {
				// A condition over an empty field cannot decide; the rule does not apply.
				continue
			}
			if !ok {
				continue
			}
		}
		for _, n := range r.Required {
			required[n] = true
		}
		for _, n := range r.Hidden {
			hidden[n] = true
		}
		for _, n := range r.ReadOnly {
			if n == meta.AllFields {
				fs.Locked = true
			}
			readOnly[n] = true
		}
		for n, v := range r.Defaults {
			fs.Defaults[n] = v
		}
	}
	for _, f := range t.Fields {
		switch {
		case required[f.Name]:
			fs.Required = append(fs.Required, f.Name)
		case hidden[f.Name]:
			// A field another rule makes required stays visible.
			fs.Hidden = append(fs.Hidden, f.Name)
		}
		if (fs.Locked || readOnly[f.Name]) && !f.Calculated() {
			fs.ReadOnly = append(fs.ReadOnly, f.Name)
		}
	}
	return fs, nil
}

func (fs FieldStates) readOnly(name string) bool {
	return fs.Locked || contains(fs.ReadOnly, name)
}

// apiShape puts written values into the shape of a read record, over a base record.
func apiShape(t meta.Type, base Record, values map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(values))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range values {
		if f, ok := t.Field(k); ok {
			out[k] = toJSON(f, v)
		} else {
			out[k] = v
		}
	}
	return out
}

// requiredProblems lists the required fields of the state that are empty.
func requiredProblems(fs FieldStates, rec map[string]any) []Problem {
	var ps []Problem
	for _, name := range fs.Required {
		if v := rec[name]; v == nil || v == "" {
			ps = append(ps, Problem{Field: name, Code: "required", Message: fmt.Sprintf("is required in state %s", fs.State)})
		}
	}
	return ps
}

// createStates applies the rules to a new record: presets of the initial state for fields the
// caller left out, no read-only field set to anything but its preset, required fields filled.
// Fields a rule set (ruled, WF4) keep the rule's value.
func createStates(t meta.Type, input, values map[string]any, ruled map[string]string) error {
	if len(t.FieldStates) == 0 {
		return nil
	}
	state := t.InitialState()
	rec := apiShape(t, Record{stateField: state}, values)
	fs, err := effective(t, state, rec)
	if err != nil {
		return err
	}
	var ps []Problem
	for name, v := range fs.Defaults {
		if _, given := input[name]; given || ruled[name] != "" {
			continue
		}
		f, _ := t.Field(name)
		cv, p := coerce(f, v)
		if p != nil {
			return fmt.Errorf("records: default of %s in the field states: %s", name, p.Message)
		}
		values[name] = cv
		rec[name] = toJSON(f, cv)
	}
	for _, f := range t.Fields {
		if _, given := input[f.Name]; !given || !fs.readOnly(f.Name) {
			continue
		}
		if d, preset := fs.Defaults[f.Name]; preset && equalJSON(rec[f.Name], d) {
			continue
		}
		ps = append(ps, Problem{Field: f.Name, Code: "read_only", Message: fmt.Sprintf("is read-only in state %s", state)})
	}
	ps = append(ps, requiredProblems(fs, rec)...)
	if len(ps) > 0 {
		return &ValidationError{Problems: ps}
	}
	return nil
}

// changeStates applies the rules to a change of a record (old) to after: fields that change
// must not be read-only in the record's state now (nor the roles, when the record is locked);
// the state after (the target of a transition, or the same) must find its required fields.
// Fields a rule set (ruled, WF4) are the system's and pass the read-only check.
func changeStates(t meta.Type, old Record, after map[string]any, actorsChange bool, ruled map[string]string) error {
	if len(t.FieldStates) == 0 {
		return nil
	}
	from, _ := old[stateField].(string)
	to, _ := after[stateField].(string)
	fsFrom, err := effective(t, from, old)
	if err != nil {
		return err
	}
	var ps []Problem
	for _, f := range t.Fields {
		if fsFrom.readOnly(f.Name) && ruled[f.Name] == "" && !equalJSON(old[f.Name], after[f.Name]) {
			ps = append(ps, Problem{Field: f.Name, Code: "read_only", Message: fmt.Sprintf("is read-only in state %s", from)})
		}
	}
	if actorsChange && fsFrom.Locked {
		ps = append(ps, Problem{Field: actorsField, Code: "read_only", Message: fmt.Sprintf("the roles are read-only in state %s", from)})
	}
	fsTo, err := effective(t, to, after)
	if err != nil {
		return err
	}
	ps = append(ps, requiredProblems(fsTo, after)...)
	if len(ps) > 0 {
		return &ValidationError{Problems: ps}
	}
	return nil
}

// FieldStates tells the form what the rules say: for a record (id), with draft values over
// it, or for a new record of the type from draft values alone.
func (s *Service) FieldStates(ctx context.Context, typeName string, id *uuid.UUID, draft map[string]any) (FieldStates, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return FieldStates{}, err
	}
	base := Record{stateField: t.InitialState()}
	if id != nil {
		if base, err = s.Get(ctx, typeName, *id); err != nil {
			return FieldStates{}, err
		}
	}
	rec := map[string]any(base)
	if len(draft) > 0 {
		draft, _, _ = splitActors(draft)
		draft, _, _ = splitTranslations(ctx, t, draft)
		values := map[string]any{}
		for name, v := range draft {
			if f, ok := t.Field(name); ok {
				if cv, p := coerce(f, v); p == nil {
					values[name] = cv
				}
			}
		}
		rec = apiShape(t, base, values)
	}
	state, _ := rec[stateField].(string)
	return effective(t, state, rec)
}
