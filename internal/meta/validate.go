// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"group24.de/itsm/internal/rules"
)

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// Problem is one validation finding with the place it belongs to.
type Problem struct {
	Where string // "package", "type probe", "type probe, field kind"
	Msg   string
}

func (p Problem) Error() string { return p.Where + ": " + p.Msg }

// ValidationError collects every problem of a package, so that one run shows them all.
type ValidationError struct {
	Package  string
	Problems []Problem
}

func (e *ValidationError) Error() string {
	lines := make([]string, 0, len(e.Problems)+1)
	lines = append(lines, fmt.Sprintf("package %q is not valid (%d problems):", e.Package, len(e.Problems)))
	for _, p := range e.Problems {
		lines = append(lines, "  - "+p.Error())
	}
	return strings.Join(lines, "\n")
}

// Validate checks a loaded package. known lists qualified names of types that other installed
// packages provide, for references across packages.
func Validate(p Package, known map[string]bool) error {
	var ps []Problem
	add := func(where, format string, args ...any) {
		ps = append(ps, Problem{Where: where, Msg: fmt.Sprintf(format, args...)})
	}

	if !IsIdent(p.Name) {
		add("package", "name %q must be snake_case (a-z, 0-9, _), at most 63 characters", p.Name)
	}
	if !versionRe.MatchString(p.Version) {
		add("package", "version %q must look like 1.2.3", p.Version)
	}
	checkLabel(add, "package", "label", p.Label)
	if p.Icon != "" && !IsPackageIcon(p.Icon) {
		add("package", "icon %q is no symbol of the list; use one of %v", p.Icon, PackageIcons)
	}
	for _, d := range p.Depends {
		if !IsIdent(d) {
			add("package", "depends on %q, which is not a valid package name", d)
		}
	}
	if p.Extends != "" && !IsIdent(p.Extends) {
		add("package", "extends %q, which is not a valid package name", p.Extends)
	}
	if len(p.Types) == 0 {
		add("package", "has no types; a package needs at least one file below types/")
	}

	local := map[string]bool{}
	for _, t := range p.Types {
		local[p.Name+"."+t.Name] = true
	}
	trseen := map[string]bool{}
	for _, tr := range p.Traits {
		where := "trait " + tr.Name
		if !IsIdent(tr.Name) {
			add(where, "name %q must be snake_case (a-z, 0-9, _), at most 63 characters", tr.Name)
		}
		if _, core := CoreTrait(tr.Name); core {
			add(where, "name %q is taken by a core trait", tr.Name)
		}
		if trseen[tr.Name] {
			add(where, "defined twice")
		}
		trseen[tr.Name] = true
		checkLabel(add, where, "label", tr.Label)
		if len(tr.Fields) == 0 {
			add(where, "has no fields; a trait of a package is a bundle of fields")
		}
		fseen := map[string]bool{}
		for _, f := range tr.Fields {
			fw := where + ", field " + f.Name
			if !IsIdent(f.Name) {
				add(fw, "name %q must be snake_case (a-z, 0-9, _), at most 63 characters", f.Name)
			}
			if isSystemColumn(f.Name) {
				add(fw, "name %q is reserved for a system column", f.Name)
			}
			if fseen[f.Name] {
				add(fw, "defined twice")
			}
			fseen[f.Name] = true
			checkLabel(add, fw, "label", f.Label)
			validateField(add, fw, f, local, known)
		}
		if len(tr.Actors) > 0 {
			// The roles a trait brings (SD-Plan N6), checked like those of a type.
			validateActors(add, where, Type{Traits: []string{TraitActors}, Actors: tr.Actors}, local, known)
		}
	}
	seen := map[string]bool{}
	for _, t := range p.Types {
		where := "type " + t.Name
		if !IsIdent(t.Name) {
			add(where, "name %q must be snake_case (a-z, 0-9, _), at most 63 characters", t.Name)
		}
		if seen[t.Name] {
			add(where, "defined twice")
		}
		seen[t.Name] = true
		checkLabel(add, where, "label", t.Label)
		if len(t.Fields) == 0 {
			add(where, "has no fields")
		}
		if t.TitleField != "" {
			if f, ok := t.Field(t.TitleField); !ok {
				add(where, "title_field %q is not a field of this type", t.TitleField)
			} else if f.Type != FieldText {
				add(where, "title_field %q must be a text field, it is %s", t.TitleField, f.Type)
			}
		}
		fseen := map[string]bool{}
		for _, f := range t.Fields {
			fw := where + ", field " + f.Name
			if !IsIdent(f.Name) {
				add(fw, "name %q must be snake_case (a-z, 0-9, _), at most 63 characters", f.Name)
			}
			if isSystemColumn(f.Name) {
				add(fw, "name %q is reserved for a system column", f.Name)
			}
			if fseen[f.Name] {
				add(fw, "defined twice")
			}
			fseen[f.Name] = true
			checkLabel(add, fw, "label", f.Label)
			validateField(add, fw, f, local, known)
		}
		validateStates(add, where, t)
		validateTraits(add, where, t, p)
		validateActors(add, where, t, local, known)
		validateResolution(add, where, t)
		validatePortal(add, where, t)
		validateReaders(add, where, t)
		validateSuggest(add, where, t, p, local, known, trseen)
		validateApproval(add, where, t)
		validateOrder(add, where, t)
		validateIndexes(add, where, t)
		validateFieldStates(add, where, t)
		validateList(add, where, t)
		validateFormulas(add, where, t)
		validateRollups(add, where, t, p)
	}
	rseen := map[string]bool{}
	for _, k := range p.Relations {
		where := "relation " + k.Name
		if !IsIdent(k.Name) {
			add(where, "name %q must be snake_case (a-z, 0-9, _), at most 63 characters", k.Name)
		}
		if rseen[k.Name] {
			add(where, "defined twice")
		}
		rseen[k.Name] = true
		checkLabel(add, where, "label", k.Label)
		checkLabel(add, where, "inverse", k.Inverse)
		if k.Update != "" && k.Update != RelationUpdateFrom {
			add(where, "update must be %q or left out (both ends), not %q", RelationUpdateFrom, k.Update)
		}
		for _, side := range []struct {
			what  string
			types []string
		}{{"from", k.From}, {"to", k.To}} {
			if len(side.types) == 0 {
				add(where, "%s needs at least one type, or *", side.what)
			}
			for _, name := range side.types {
				if trait, ok := strings.CutPrefix(name, TraitPrefix); ok {
					// A trait of this package or of one it depends on (SD-Plan N11).
					pkg, tr, dotted := strings.Cut(trait, ".")
					switch {
					case !dotted:
						add(where, "%s names %q; a trait goes with its package, as trait:<package>.<trait>", side.what, name)
					case pkg == p.Name && !trseen[tr]:
						add(where, "%s names %q, which is no trait of this package", side.what, name)
					case pkg != p.Name && !slices.Contains(p.Depends, pkg):
						add(where, "%s names %q of package %s, which this package does not depend on", side.what, name, pkg)
					}
					continue
				}
				if name != "*" && !local[name] && !known[name] {
					add(where, "%s names %q, which is neither a type of this package nor installed", side.what, name)
				}
			}
		}
	}
	validateData(p, local, known, add)
	checkSettings(add, p)
	if len(ps) == 0 {
		return nil
	}
	return &ValidationError{Package: p.Name, Problems: ps}
}

// validateFieldStates checks the field-state rules (WF2): known states and fields, a condition
// that compiles, "*" only for read_only, no field both required and hidden in one rule,
// defaults of the field's kind, calculated fields neither required nor preset.
func validateFieldStates(add func(string, string, ...any), where string, t Type) {
	states := map[string]bool{}
	for _, s := range t.States {
		states[s.Name] = true
	}
	for i, r := range t.FieldStates {
		rw := fmt.Sprintf("%s, field_states[%d]", where, i)
		if len(r.Required)+len(r.Hidden)+len(r.ReadOnly)+len(r.Defaults) == 0 {
			add(rw, "says nothing; give required, hidden, read_only or defaults")
		}
		for _, s := range r.States {
			if !states[s] {
				add(rw, "state %q is not a state of this type", s)
			}
		}
		if r.When != "" {
			if _, err := rules.Compile(r.When); err != nil {
				add(rw, "when: %v", err)
			}
		}
		check := func(what string, names []string, star bool) map[string]bool {
			set := map[string]bool{}
			for _, name := range names {
				set[name] = true
				if name == AllFields {
					if !star || len(names) > 1 {
						add(rw, "%s: * stands alone and only in read_only", what)
					}
					continue
				}
				f, ok := t.Field(name)
				switch {
				case !ok:
					add(rw, "%s: %q is not a field of this type", what, name)
				case f.Calculated() && what != "hidden":
					add(rw, "%s: %q is calculated by the core", what, name)
				}
			}
			return set
		}
		required := check("required", r.Required, false)
		for name := range check("hidden", r.Hidden, false) {
			if required[name] {
				add(rw, "%q cannot be required and hidden", name)
			}
		}
		check("read_only", r.ReadOnly, true)
		for name, v := range r.Defaults {
			f, ok := t.Field(name)
			if !ok || f.Calculated() {
				add(rw, "defaults: %q is not a field that can be preset", name)
				continue
			}
			f.Default = v
			if msg := checkDefault(f); msg != "" {
				add(rw, "defaults: %s: %s", name, msg)
			} else if f.Type == FieldChoice && !hasOption(f, v) {
				add(rw, "defaults: %s: %v is not an option", name, v)
			}
		}
	}
}

func hasOption(f Field, v any) bool {
	for _, o := range f.Options {
		if o.Value == v {
			return true
		}
	}
	return false
}

// validateResolution checks the trait resolution (UI14a): it needs a state of the category
// done to set its fields, and the history to leave the time of waiting out.
func validateResolution(add func(string, string, ...any), where string, t Type) {
	if !t.HasTrait(TraitResolution) {
		return
	}
	if !t.HasHistory() {
		add(where, "trait resolution needs the trait history")
	}
	for _, st := range t.States {
		if st.Category == CategoryDone {
			return
		}
	}
	add(where, "trait resolution needs a state of the category done")
}

// validateActors checks the roles at the record (WF3): the trait actors and at least one role
// go together; each role has a snake_case name, labels and holder types that exist.
func validateActors(add func(string, string, ...any), where string, t Type, local, known map[string]bool) {
	has := t.HasTrait(TraitActors)
	switch {
	case has && len(t.Actors) == 0:
		add(where, "trait actors needs at least one role under actors")
	case !has && len(t.Actors) > 0:
		add(where, "actors needs the trait actors")
	}
	if _, clash := t.Field("actors"); has && clash {
		add(where, "a field named actors collides with the roles of the trait actors")
	}
	seen := map[string]bool{}
	for _, r := range t.Actors {
		rw := where + ", role " + r.Name
		if !IsIdent(r.Name) {
			add(rw, "name %q must be snake_case (a-z, 0-9, _), at most 63 characters", r.Name)
		}
		if seen[r.Name] {
			add(rw, "defined twice")
		}
		seen[r.Name] = true
		checkLabel(add, rw, "label", r.Label)
		if len(r.To) == 0 {
			add(rw, "to needs at least one type that may hold the role, such as core.person")
		}
		for _, name := range r.To {
			if !strings.Contains(name, ".") {
				add(rw, "to %q must be written as package.type", name)
			} else if !local[name] && !known[name] {
				add(rw, "to %q points to a type that is neither in this package nor installed", name)
			}
		}
	}
}

// validatePortal checks the fields a type offers in the portal (UI10): fields of the type that
// a user can fill, each once.
func validatePortal(add func(string, string, ...any), where string, t Type) {
	if t.Portal == nil {
		return
	}
	if len(t.Portal.Fields) == 0 && t.Portal.Read == nil {
		add(where+", portal", "fields needs at least one field to create with")
	}
	seen := map[string]bool{}
	for _, name := range t.Portal.Fields {
		f, ok := t.Field(name)
		switch {
		case !ok:
			add(where+", portal", "field %q is not a field of the type", name)
		case f.Calculated():
			add(where+", portal", "field %q is kept by the server and cannot be filled", name)
		case seen[name]:
			add(where+", portal", "field %q is named twice", name)
		}
		seen[name] = true
	}
	if r := t.Portal.Read; r != nil {
		// The reading area (SD16a): a name for the head, fields to show, a reference to group by.
		rw := where + ", portal read"
		checkLabel(add, rw, "label", r.Label)
		if r.Icon != "" && !IsPackageIcon(r.Icon) {
			add(rw, "icon %q is no symbol of the list; use one of %v", r.Icon, PackageIcons)
		}
		if len(r.Fields) == 0 {
			add(rw, "fields needs at least one field to show")
		}
		seen := map[string]bool{}
		for _, name := range r.Fields {
			if _, ok := t.Field(name); !ok {
				add(rw, "field %q is not a field of the type", name)
			} else if seen[name] {
				add(rw, "field %q is named twice", name)
			}
			seen[name] = true
		}
		if r.Group != "" {
			if f, ok := t.Field(r.Group); !ok || f.Type != FieldReference {
				add(rw, "group %q must be a reference field of the type", r.Group)
			}
		}
		if r.Sort != "" {
			if _, ok := t.Field(strings.TrimPrefix(r.Sort, "-")); !ok {
				add(rw, "sort %q is not a field of the type", r.Sort)
			}
		}
		seen = map[string]bool{}
		for _, name := range r.Facts {
			if _, ok := t.Field(name); !ok {
				add(rw, "fact %q is not a field of the type", name)
			} else if seen[name] {
				add(rw, "fact %q is named twice", name)
			}
			seen[name] = true
		}
	}
}

// validateReaders checks the rules by which persons read without a grant (SD16a): conditions
// that compile, roles of the type. Whether a condition translates to SQL decides the access
// as for permissions; one that does not lets nobody read.
func validateReaders(add func(string, string, ...any), where string, t Type) {
	for i, r := range t.Readers {
		rw := fmt.Sprintf("%s, readers[%d]", where, i)
		if r.When != "" {
			if _, err := rules.Compile(r.When); err != nil {
				add(rw, "when: %v", err)
			}
		}
		for _, role := range r.Roles {
			if _, ok := t.Actor(role); !ok {
				add(rw, "role %q is not a role of the type", role)
			}
		}
		for _, a := range r.Actions {
			if !slices.Contains(ReaderActions, a) {
				add(rw, "action %q must be one of %s", a, strings.Join(ReaderActions, ", "))
			}
			if a == "comment" && !t.HasTrait(TraitTimeline) {
				add(rw, "action comment needs the trait timeline")
			}
		}
	}
}

// validateStates checks the state model (WF1): snake_case names, exactly one initial state,
// transitions between known states, conditions that compile.
func validateStates(add func(string, string, ...any), where string, t Type) {
	if len(t.States) == 0 {
		if len(t.Transitions) != 0 {
			add(where, "transitions need states")
		}
		return
	}
	states := map[string]bool{}
	initial := 0
	for _, s := range t.States {
		sw := where + ", state " + s.Name
		if !IsIdent(s.Name) {
			add(sw, "name %q must be snake_case (a-z, 0-9, _), at most 63 characters", s.Name)
		}
		if states[s.Name] {
			add(sw, "defined twice")
		}
		states[s.Name] = true
		checkLabel(add, sw, "label", s.Label)
		switch s.Category {
		case "", CategoryOpen, CategoryActive, CategoryWaiting, CategoryDone:
		default:
			add(sw, "category %q must be open, active, waiting or done", s.Category)
		}
		if s.Initial {
			initial++
		}
	}
	if initial != 1 {
		add(where, "exactly one state must be marked initial, %d are", initial)
	}
	for _, name := range slices.Sorted(maps.Keys(t.Layout)) {
		if !states[name] {
			add(where, "layout names %q, which is not a state of this type", name)
		}
	}
	tseen := map[string]bool{}
	for _, tr := range t.Transitions {
		tw := where + ", transition " + tr.Name
		if !IsIdent(tr.Name) {
			add(tw, "name %q must be snake_case (a-z, 0-9, _), at most 63 characters", tr.Name)
		}
		if tseen[tr.Name] {
			add(tw, "defined twice")
		}
		tseen[tr.Name] = true
		checkLabel(add, tw, "label", tr.Label)
		if len(tr.From) == 0 {
			add(tw, "needs at least one state in \"from\"")
		}
		for _, f := range tr.From {
			if !states[f] {
				add(tw, "from %q is not a state of this type", f)
			}
		}
		if !states[tr.To] {
			add(tw, "to %q is not a state of this type", tr.To)
		}
		if tr.When != "" {
			if _, err := rules.Compile(tr.When); err != nil {
				add(tw, "when: %v", err)
			}
		}
		// Who alone may take it, and whether it takes a text (SD-Plan N8).
		for _, role := range tr.By {
			if _, ok := t.Actor(role); !ok {
				add(tw, "by %q is not a role at the record of this type", role)
			}
		}
		if tr.Comment != "" && tr.Comment != CommentRequired && tr.Comment != CommentOptional {
			add(tw, "comment %q must be required or optional", tr.Comment)
		}
		// Fixed values (SD5): fields of the type that can take them, of their kind.
		for _, name := range slices.Sorted(maps.Keys(tr.Set)) {
			f, ok := t.Field(name)
			if !ok || f.Calculated() {
				add(tw, "set: %q is not a field that can be set", name)
				continue
			}
			f.Default = tr.Set[name]
			if msg := checkDefault(f); msg != "" {
				add(tw, "set: %s: %s", name, msg)
			} else if f.Type == FieldChoice && !hasOption(f, tr.Set[name]) {
				add(tw, "set: %s: %v is not an option", name, tr.Set[name])
			}
		}
		if len(tr.Set) > 0 && len(tr.By) > 0 {
			add(tw, "a transition taken by a role sets no fields")
		}
	}
}

func validateField(add func(string, string, ...any), fw string, f Field, local, known map[string]bool) {
	typeOK := false
	for _, ft := range FieldTypes {
		if f.Type == ft {
			typeOK = true
		}
	}
	if !typeOK {
		add(fw, "unknown field type %q, allowed are %s", f.Type, joinTypes())
		return
	}
	if f.Type != FieldText && f.MaxLength != 0 {
		add(fw, "max_length only applies to text fields")
	}
	rich := f.Type == FieldText && f.Format == FormatRich
	priority := f.Type == FieldNumber && f.Format == FormatPriority
	icon := f.Type == FieldText && f.Format == FormatIcon
	if f.Format != "" && !rich && !priority && !icon {
		add(fw, `format takes "rich" or "icon" on text fields and "priority" on number fields`)
	}
	if f.Type != FieldSequence && f.Prefix != "" {
		add(fw, "prefix only applies to sequence fields")
	}
	if f.Snippets != "" && (!rich || !IsIdent(f.Snippets)) {
		add(fw, "snippets takes a use like solution on formatted text fields")
	}
	if f.Type == FieldSequence {
		if len(f.Prefix) > 20 || strings.ContainsAny(f.Prefix, " \t\n") {
			add(fw, "prefix must be at most 20 characters without spaces")
		}
		if f.Required || f.Default != nil || f.Formula != "" {
			add(fw, "a sequence field is set by the core: no required, default or formula")
		}
	}
	if f.Type != FieldText && f.Translatable {
		add(fw, "translatable only applies to text fields")
	}
	if f.MaxLength < 0 {
		add(fw, "max_length must not be negative")
	}
	if f.Type != FieldNumber && (f.Min != nil || f.Max != nil) {
		add(fw, "min and max only apply to number fields")
	}
	if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
		add(fw, "min %v is greater than max %v", *f.Min, *f.Max)
	}
	switch f.Type {
	case FieldChoice:
		if len(f.Options) == 0 {
			add(fw, "a choice field needs options")
		}
		oseen := map[string]bool{}
		for _, o := range f.Options {
			if !IsIdent(o.Value) {
				add(fw, "option value %q must be snake_case", o.Value)
			}
			if oseen[o.Value] {
				add(fw, "option %q listed twice", o.Value)
			}
			oseen[o.Value] = true
			checkLabel(add, fw+", option "+o.Value, "label", o.Label)
		}
		if s, ok := f.Default.(string); ok && !oseen[s] {
			add(fw, "default %q is not one of the options", s)
		}
	case FieldReference:
		switch {
		case f.To == "":
			add(fw, "a reference field needs \"to: package.type\"")
		case !strings.Contains(f.To, "."):
			add(fw, "to %q must be written as package.type", f.To)
		case !local[f.To] && !known[f.To]:
			add(fw, "to %q points to a type that is neither in this package nor installed", f.To)
		}
	default:
		if len(f.Options) != 0 {
			add(fw, "options only apply to choice fields")
		}
		if f.To != "" {
			add(fw, "\"to\" only applies to reference fields")
		}
	}
	if f.Default != nil {
		if msg := checkDefault(f); msg != "" {
			add(fw, "%s", msg)
		}
	}
}

func checkDefault(f Field) string {
	switch f.Type {
	case FieldText, FieldChoice, FieldDate, FieldDateTime, FieldReference:
		if _, ok := f.Default.(string); !ok {
			return fmt.Sprintf("default must be a string for a %s field", f.Type)
		}
	case FieldNumber:
		switch f.Default.(type) {
		case int, int64, uint64, float64:
		default:
			return "default must be a number for a number field"
		}
	case FieldBoolean:
		if _, ok := f.Default.(bool); !ok {
			return "default must be true or false for a boolean field"
		}
	}
	return ""
}

func checkLabel(add func(string, string, ...any), where, what string, l Label) {
	if len(l) == 0 {
		add(where, "%s is missing; it needs %s", what, strings.Join(Languages, " and "))
		return
	}
	for _, lang := range Languages {
		if strings.TrimSpace(l[lang]) == "" {
			add(where, "%s has no %q text", what, lang)
		}
	}
}

func joinTypes() string {
	s := make([]string, len(FieldTypes))
	for i, t := range FieldTypes {
		s[i] = string(t)
	}
	return strings.Join(s, ", ")
}

// systemColumns are the columns every type table has (architektur.md §3.1); fields cannot use them.
var systemColumns = map[string]bool{
	"id": true, "org_path": true, "version": true, "state": true, "source": true, "external_key": true,
	"row_hash": true, "search": true, "created_at": true, "created_by": true, "updated_at": true,
	"updated_by": true, "deleted_at": true, "sla_due_at": true, "sla_warn_at": true, "sla_paused": true,
	// The tenant of the row (PL46); missing here until 2026-11-03, found in UI34.
	"tenant": true,
}

func isSystemColumn(name string) bool { return systemColumns[name] }

// AsValidationError returns the validation error inside err, if any.
func AsValidationError(err error) (*ValidationError, bool) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}
