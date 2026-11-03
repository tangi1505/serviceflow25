// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/db/queries"
)

// The customer layer (PL43, Hauptplan E135, E37; architektur.md §4) holds what was changed in
// the interface as a list of changes per type. The effective type is the base type with the
// active changes applied in order. Like an extension, the customer layer adds and narrowly
// changes: its own fields (named k_…) freely; fields of the package only in label,
// description, required, default, order and options added, and hidden, never removed,
// renamed or retyped. A change that no longer fits the base (a package update removed its
// field) is a conflict; it does not apply, and a forced update marks it inactive.

// CustomerPrefix starts the name of every field the customer layer adds; packages may not use
// it, so an update can never bring a field of the same name.
const CustomerPrefix = "k_"

// Operations of the customer layer.
const (
	OpAddField    = "add_field"
	OpChangeField = "change_field"
	OpAddOptions  = "add_options"
	OpHideField   = "hide_field"
	OpShowField   = "show_field"
	OpRemoveField = "remove_field"
	OpMoveField   = "move_field"
	// The state model (UI26.1): own states and transitions (k_…) freely; states of the package
	// only in label and initial, transitions of the package in label, from, to, when and
	// disabled.
	OpAddState          = "add_state"
	OpChangeState       = "change_state"
	OpRemoveState       = "remove_state"
	OpAddTransition     = "add_transition"
	OpChangeTransition  = "change_transition"
	OpRemoveTransition  = "remove_transition"
	OpDisableTransition = "disable_transition"
	OpEnableTransition  = "enable_transition"
	// The graph (UI26.2): the positions of all states at once, as one change.
	OpSetLayout = "set_layout"
)

// Change is one change of the customer layer.
type Change struct {
	ID    string `json:"id"`
	Op    string `json:"op"`
	Field string `json:"field"`
	// New is the field of add_field.
	New *Field `json:"new,omitempty"`
	// State is the state of add_state; Transition the transition of add_transition.
	State      *State      `json:"state,omitempty"`
	Transition *Transition `json:"transition,omitempty"`
	// Layout is the whole layout of the graph set_layout replaces (field is "layout").
	Layout map[string]Point `json:"layout,omitempty"`
	// Set are the attributes change_field, change_state and change_transition set.
	Set *FieldPatch `json:"set,omitempty"`
	// Options are what add_options adds.
	Options []Option `json:"options,omitempty"`
	// After names the field move_field puts it behind; empty puts it first.
	After string    `json:"after,omitempty"`
	At    time.Time `json:"at"`
	By    string    `json:"by,omitempty"`
	// Inactive is why a forced package update stilled the change; it then does not apply.
	Inactive string `json:"inactive,omitempty"`
}

// FieldPatch are the attributes the customer layer may set: of a field (label, description,
// required, default; max_length only on its own fields), of a state (label, initial) and of
// a transition (label, from, to, when; an empty when removes the condition).
type FieldPatch struct {
	Label       Label    `json:"label,omitempty"`
	Description Label    `json:"description,omitempty"`
	Required    *bool    `json:"required,omitempty"`
	Default     any      `json:"default,omitempty"`
	MaxLength   *int     `json:"max_length,omitempty"`
	Initial     *bool    `json:"initial,omitempty"`
	From        []string `json:"from,omitempty"`
	To          string   `json:"to,omitempty"`
	When        *string  `json:"when,omitempty"`
}

// CustomerLayer is the list of changes of one type.
type CustomerLayer struct {
	Changes []Change `json:"changes"`
}

// Conflict is a change that does not apply to the base, with the reason.
type Conflict struct {
	Change Change `json:"change"`
	Reason string `json:"reason"`
}

// own says whether the customer layer added the field.
func own(name string) bool { return strings.HasPrefix(name, CustomerPrefix) }

// ApplyCustomer applies the active changes of the layer to the base type in order; it returns
// the effective type and the changes that did not apply. Inactive changes are skipped.
func ApplyCustomer(base Type, layer CustomerLayer) (Type, []Conflict) {
	t := base
	t.Fields = slices.Clone(base.Fields)
	t.States = slices.Clone(base.States)
	t.Transitions = slices.Clone(base.Transitions)
	t.Layout = maps.Clone(base.Layout)
	var conflicts []Conflict
	for _, c := range layer.Changes {
		if c.Inactive != "" {
			continue
		}
		if reason := applyChange(&t, c); reason != "" {
			conflicts = append(conflicts, Conflict{Change: c, Reason: reason})
		}
	}
	return t, conflicts
}

// applyChange applies one change to t, or says why it cannot.
func applyChange(t *Type, c Change) string {
	switch c.Op {
	case OpAddState, OpChangeState, OpRemoveState:
		return applyState(t, c)
	case OpAddTransition, OpChangeTransition, OpRemoveTransition, OpDisableTransition, OpEnableTransition:
		return applyTransition(t, c)
	case OpSetLayout:
		return applyLayout(t, c)
	}
	i := slices.IndexFunc(t.Fields, func(f Field) bool { return f.Name == c.Field })
	if c.Op != OpAddField && i < 0 {
		return fmt.Sprintf("%s has no field %s (any more)", t.QualifiedName(), c.Field)
	}
	switch c.Op {
	case OpAddField:
		switch {
		case c.New == nil || c.New.Name != c.Field:
			return "add_field needs the new field under new, with the name of field"
		case !own(c.Field):
			return fmt.Sprintf("a field of the customer layer is named %s…", CustomerPrefix)
		case i >= 0:
			return fmt.Sprintf("%s has a field %s already", t.QualifiedName(), c.Field)
		case c.New.Formula != "" || c.New.Rollup != nil || c.New.Computed || c.New.Trait != "" || c.New.Hidden:
			return "a field of the customer layer has no formula, rollup or trait"
		}
		f := *c.New
		f.Label, f.Description = withEnglish(f.Label), withEnglish(f.Description)
		f.Options = slices.Clone(f.Options)
		for k := range f.Options {
			f.Options[k].Label = withEnglish(f.Options[k].Label)
		}
		t.Fields = append(t.Fields, f)
	case OpChangeField:
		if c.Set == nil {
			return "change_field needs set"
		}
		f := t.Fields[i]
		if c.Set.MaxLength != nil {
			if !own(f.Name) {
				return fmt.Sprintf("max_length of %s belongs to the package", f.Name)
			}
			f.MaxLength = *c.Set.MaxLength
		}
		f.Label = mergeLabel(f.Label, c.Set.Label)
		f.Description = mergeLabel(f.Description, c.Set.Description)
		if c.Set.Required != nil {
			f.Required = *c.Set.Required
		}
		if f.Hidden && f.Required && f.Default == nil {
			return fmt.Sprintf("%s is hidden: required needs a default", f.Name)
		}
		if c.Set.Default != nil {
			f.Default = c.Set.Default
			if msg := checkDefault(f); msg != "" {
				return msg
			}
			if f.Type == FieldChoice && !hasOption(f, f.Default) {
				return fmt.Sprintf("default %v is not one of the options", f.Default)
			}
		}
		t.Fields[i] = f
	case OpAddOptions:
		f := t.Fields[i]
		if f.Type != FieldChoice {
			return fmt.Sprintf("%s is no choice field", f.Name)
		}
		if len(c.Options) == 0 {
			return "add_options needs options"
		}
		f.Options = slices.Clone(f.Options)
		for _, o := range c.Options {
			if !IsIdent(o.Value) {
				return fmt.Sprintf("option value %q must be snake_case", o.Value)
			}
			if hasOption(f, o.Value) {
				return fmt.Sprintf("%s has the option %s already", f.Name, o.Value)
			}
			o.Label = withEnglish(o.Label)
			f.Options = append(f.Options, o)
		}
		t.Fields[i] = f
	case OpHideField, OpShowField:
		if c.Field == t.TitleField && c.Op == OpHideField {
			return fmt.Sprintf("%s is the title of %s and stays visible", c.Field, t.QualifiedName())
		}
		if f := t.Fields[i]; c.Op == OpHideField && f.Required && f.Default == nil {
			return fmt.Sprintf("%s is required without a default: hidden, nobody could create", f.Name)
		}
		t.Fields[i].Hidden = c.Op == OpHideField
	case OpRemoveField:
		if !own(c.Field) {
			return fmt.Sprintf("%s belongs to the package: hide it instead", c.Field)
		}
		t.Fields = slices.Delete(t.Fields, i, i+1)
	case OpMoveField:
		f := t.Fields[i]
		rest := slices.Delete(slices.Clone(t.Fields), i, i+1)
		at := 0
		if c.After != "" {
			j := slices.IndexFunc(rest, func(x Field) bool { return x.Name == c.After })
			if j < 0 {
				return fmt.Sprintf("%s has no field %s to move behind", t.QualifiedName(), c.After)
			}
			at = j + 1
		}
		t.Fields = slices.Insert(rest, at, f)
	default:
		return fmt.Sprintf("unknown operation %q", c.Op)
	}
	return ""
}

// applyState applies a change of a state (UI26.1): an own state comes and goes freely, one
// of the package changes in label and initial only. Whether exactly one state is initial and
// every transition fits, Validate says afterwards.
func applyState(t *Type, c Change) string {
	i := slices.IndexFunc(t.States, func(s State) bool { return s.Name == c.Field })
	switch c.Op {
	case OpAddState:
		switch {
		case c.State == nil || c.State.Name != c.Field:
			return "add_state needs the new state under state, with the name of field"
		case !own(c.Field):
			return fmt.Sprintf("a state of the customer layer is named %s…", CustomerPrefix)
		case i >= 0:
			return fmt.Sprintf("%s has a state %s already", t.QualifiedName(), c.Field)
		}
		s := *c.State
		s.Label = withEnglish(s.Label)
		if s.Initial {
			clearInitial(t)
		}
		t.States = append(t.States, s)
	case OpChangeState:
		if i < 0 {
			return fmt.Sprintf("%s has no state %s (any more)", t.QualifiedName(), c.Field)
		}
		if c.Set == nil {
			return "change_state needs set"
		}
		s := t.States[i]
		s.Label = mergeLabel(s.Label, c.Set.Label)
		if c.Set.Initial != nil {
			if !*c.Set.Initial && s.Initial {
				return fmt.Sprintf("%s is the initial state: mark another one initial instead", s.Name)
			}
			if *c.Set.Initial {
				clearInitial(t)
			}
			s.Initial = *c.Set.Initial
		}
		t.States[i] = s
	case OpRemoveState:
		if i < 0 {
			return fmt.Sprintf("%s has no state %s (any more)", t.QualifiedName(), c.Field)
		}
		if !own(c.Field) {
			return fmt.Sprintf("%s belongs to the package and stays", c.Field)
		}
		if t.States[i].Initial {
			return fmt.Sprintf("%s is the initial state: mark another one initial first", c.Field)
		}
		for _, tr := range t.Transitions {
			if tr.To == c.Field || slices.Contains(tr.From, c.Field) {
				return fmt.Sprintf("transition %s still uses %s", tr.Name, c.Field)
			}
		}
		t.States = slices.Delete(t.States, i, i+1)
		if t.Layout != nil {
			t.Layout = maps.Clone(t.Layout)
			delete(t.Layout, c.Field)
		}
	}
	return ""
}

// applyLayout replaces the positions of the states in the graph of the workflow editor
// (UI26.2, UI-Plan E27): every name must be a state; an empty layout lets the editor
// arrange anew. A state that goes (remove_state) leaves the layout with it.
func applyLayout(t *Type, c Change) string {
	if c.Layout == nil {
		return "set_layout needs layout"
	}
	for _, name := range slices.Sorted(maps.Keys(c.Layout)) {
		if !slices.ContainsFunc(t.States, func(s State) bool { return s.Name == name }) {
			return fmt.Sprintf("layout names %s, which is not a state of %s", name, t.QualifiedName())
		}
	}
	if len(c.Layout) == 0 {
		t.Layout = nil
		return ""
	}
	t.Layout = maps.Clone(c.Layout)
	return ""
}

// clearInitial takes the mark initial off every state, before another one gets it.
func clearInitial(t *Type) {
	for k := range t.States {
		t.States[k].Initial = false
	}
}

// applyTransition applies a change of a transition (UI26.1): an own transition comes and
// goes freely; one of the package changes in label, from, to and when, and is disabled
// instead of removed. From and to must be states of the type, when must compile: Validate.
func applyTransition(t *Type, c Change) string {
	i := slices.IndexFunc(t.Transitions, func(tr Transition) bool { return tr.Name == c.Field })
	if c.Op != OpAddTransition && i < 0 {
		return fmt.Sprintf("%s has no transition %s (any more)", t.QualifiedName(), c.Field)
	}
	switch c.Op {
	case OpAddTransition:
		switch {
		case c.Transition == nil || c.Transition.Name != c.Field:
			return "add_transition needs the new transition under transition, with the name of field"
		case !own(c.Field):
			return fmt.Sprintf("a transition of the customer layer is named %s…", CustomerPrefix)
		case i >= 0:
			return fmt.Sprintf("%s has a transition %s already", t.QualifiedName(), c.Field)
		}
		tr := *c.Transition
		tr.Label = withEnglish(tr.Label)
		tr.From = slices.Clone(tr.From)
		tr.Disabled = false
		t.Transitions = append(t.Transitions, tr)
	case OpChangeTransition:
		if c.Set == nil {
			return "change_transition needs set"
		}
		tr := t.Transitions[i]
		tr.Label = mergeLabel(tr.Label, c.Set.Label)
		if len(c.Set.From) > 0 {
			tr.From = slices.Clone(c.Set.From)
		}
		if c.Set.To != "" {
			tr.To = c.Set.To
		}
		if c.Set.When != nil {
			tr.When = *c.Set.When
		}
		t.Transitions[i] = tr
	case OpRemoveTransition:
		if !own(c.Field) {
			return fmt.Sprintf("%s belongs to the package: disable it instead", c.Field)
		}
		t.Transitions = slices.Delete(t.Transitions, i, i+1)
	case OpDisableTransition, OpEnableTransition:
		t.Transitions[i].Disabled = c.Op == OpDisableTransition
	}
	return ""
}

// withEnglish fills a missing English text with the German one: the interface may write
// German only, the definitions carry both (PL21).
func withEnglish(l Label) Label {
	if len(l) == 0 || l["en"] != "" || l["de"] == "" {
		return l
	}
	out := Label{}
	for k, v := range l {
		out[k] = v
	}
	out["en"] = l["de"]
	return out
}

// mergeLabel sets the languages the patch names; an empty text removes that language.
func mergeLabel(l, patch Label) Label {
	if len(patch) == 0 {
		return l
	}
	out := Label{}
	for k, v := range l {
		out[k] = v
	}
	for k, v := range patch {
		if v == "" {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

// revision is the hash of the stored layers of a type, the version of its effective
// definition for caches and clients.
func revision(base, customer []byte) string {
	h := sha256.New()
	h.Write(base)
	h.Write([]byte{0})
	h.Write(customer)
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// ValidateCustomer checks the effective type after a change as a package would be checked,
// except for the prefix of the customer layer; known are the installed types. The type is
// checked inside its package: the other types (base) and traits of the package count, as at
// installation, so that a type with traits or rollups of its package can be changed (found
// with UI25 on probe.probe).
func ValidateCustomer(t Type, base []Type, traits []Trait, known map[string]bool) error {
	p := Package{Name: t.Package, Version: "0.0.0", Label: Label{"de": t.Package, "en": t.Package}, Types: []Type{t}}
	for _, o := range base {
		if o.Package == t.Package && o.Name != t.Name {
			p.Types = append(p.Types, o)
		}
	}
	for _, tr := range traits {
		if tr.Package == t.Package {
			p.Traits = append(p.Traits, tr)
		}
	}
	return Validate(p, known)
}

// checkPrefix refuses fields of a package that use the prefix of the customer layer.
func checkPrefix(p Package) error {
	var ps []Problem
	for _, t := range p.Types {
		for _, f := range t.Fields {
			if own(f.Name) && f.Trait == "" {
				ps = append(ps, Problem{Where: "type " + t.Name + ", field " + f.Name,
					Msg: fmt.Sprintf("the prefix %s belongs to the customer layer", CustomerPrefix)})
			}
		}
	}
	for _, tr := range p.Traits {
		for _, f := range tr.Fields {
			if own(f.Name) {
				ps = append(ps, Problem{Where: "trait " + tr.Name + ", field " + f.Name,
					Msg: fmt.Sprintf("the prefix %s belongs to the customer layer", CustomerPrefix)})
			}
		}
	}
	if len(ps) > 0 {
		return &ValidationError{Problems: ps}
	}
	return nil
}

// ErrLayerChanged is a customer layer saved by someone else in between.
var ErrLayerChanged = errors.New("meta: the customer layer changed in between")

// GetCustomerLayer reads the customer layer of a type and its version; 0 means none yet.
func GetCustomerLayer(ctx context.Context, q *queries.Queries, pkg, name string) (CustomerLayer, int64, error) {
	row, err := q.GetDefinition(ctx, queries.GetDefinitionParams{Package: pkg, Kind: KindType, Name: name, Layer: LayerCustomer})
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomerLayer{}, 0, nil
	}
	if err != nil {
		return CustomerLayer{}, 0, fmt.Errorf("meta: customer layer %s.%s: %w", pkg, name, err)
	}
	var l CustomerLayer
	if err := json.Unmarshal(row.Spec, &l); err != nil {
		return CustomerLayer{}, 0, fmt.Errorf("meta: customer layer %s.%s: %w", pkg, name, err)
	}
	return l, row.Version, nil
}

// SaveCustomerLayer writes the layer of a type if it still has the expected version (0: none
// yet) and returns the new version.
func SaveCustomerLayer(ctx context.Context, q *queries.Queries, pkg, name string, l CustomerLayer, expected int64) (int64, error) {
	if l.Changes == nil {
		l.Changes = []Change{}
	}
	spec, err := json.Marshal(l)
	if err != nil {
		return 0, err
	}
	if expected == 0 {
		id, err := uuid.NewV7()
		if err != nil {
			return 0, err
		}
		n, err := q.InsertDefinition(ctx, queries.InsertDefinitionParams{ID: id, Package: pkg, Kind: KindType, Name: name, Layer: LayerCustomer, Spec: spec})
		if err != nil {
			return 0, fmt.Errorf("meta: customer layer %s.%s: %w", pkg, name, err)
		}
		if n == 0 {
			return 0, ErrLayerChanged
		}
		return 1, nil
	}
	n, err := q.UpdateDefinitionVersioned(ctx, queries.UpdateDefinitionVersionedParams{
		Package: pkg, Kind: KindType, Name: name, Layer: LayerCustomer, Spec: spec, Version: expected,
	})
	if err != nil {
		return 0, fmt.Errorf("meta: customer layer %s.%s: %w", pkg, name, err)
	}
	if n == 0 {
		return 0, ErrLayerChanged
	}
	return expected + 1, nil
}

// CustomerLayers returns every customer layer by qualified type name.
func CustomerLayers(ctx context.Context, q *queries.Queries) (map[string]CustomerLayer, map[string]int64, error) {
	rows, err := q.ListLayerDefinitions(ctx, queries.ListLayerDefinitionsParams{Kind: KindType, Layer: LayerCustomer})
	if err != nil {
		return nil, nil, fmt.Errorf("meta: customer layers: %w", err)
	}
	layers := map[string]CustomerLayer{}
	versions := map[string]int64{}
	for _, r := range rows {
		var l CustomerLayer
		if err := json.Unmarshal(r.Spec, &l); err != nil {
			return nil, nil, fmt.Errorf("meta: customer layer %s.%s: %w", r.Package, r.Name, err)
		}
		layers[r.Package+"."+r.Name] = l
		versions[r.Package+"."+r.Name] = r.Version
	}
	return layers, versions, nil
}
