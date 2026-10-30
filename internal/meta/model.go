// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package meta holds the meta-model: packages, entity types and their fields
// (architektur.md §2 "Meta-Modell", §4; Hauptplan E7, E37, E66 to E68).
//
// A package is a directory with package.yaml and types/<name>.yaml. Technical names are
// snake_case and unique per package; display names are translated labels.
package meta

import (
	"regexp"
	"sort"
)

// Label is a display name per language, e.g. {"de": "Störung", "en": "Incident"}.
type Label map[string]string

// Languages the core ships (Hauptplan E42). Every label must have both.
var Languages = []string{"de", "en"}

// DefaultLanguage is the language of the columns and the fallback of translated values
// (Hauptplan E98, PL21).
const DefaultLanguage = "de"

// IsLanguage reports whether the core knows the language.
func IsLanguage(code string) bool {
	for _, l := range Languages {
		if l == code {
			return true
		}
	}
	return false
}

// identRe is the rule for technical names of packages, types, fields and choice values.
var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// IsIdent reports whether s is a valid technical name.
func IsIdent(s string) bool { return identRe.MatchString(s) }

// FieldType is the kind of a field; each maps to one PostgreSQL column type (architektur.md §3.1).
type FieldType string

const (
	FieldText      FieldType = "text"      // text
	FieldNumber    FieldType = "number"    // numeric
	FieldBoolean   FieldType = "boolean"   // boolean
	FieldDate      FieldType = "date"      // date
	FieldDateTime  FieldType = "datetime"  // timestamptz
	FieldChoice    FieldType = "choice"    // text, restricted to Options
	FieldReference FieldType = "reference" // uuid, points to a record of type To
)

// Portal says how a type appears in the portal (UI10): the fields a user fills when creating
// and sees on the page of a record.
type Portal struct {
	Fields []string `json:"fields" yaml:"fields"`
}

// FormatRich marks a text field as formatted text (UI7).
const FormatRich = "rich"

// FieldTypes lists the supported kinds in a stable order (for messages).
var FieldTypes = []FieldType{FieldText, FieldNumber, FieldBoolean, FieldDate, FieldDateTime, FieldChoice, FieldReference}

// Option is one allowed value of a choice field.
type Option struct {
	Value string `json:"value" yaml:"value"`
	Label Label  `json:"label" yaml:"label"`
}

// Field is one attribute of an entity type.
type Field struct {
	Name        string    `json:"name" yaml:"name"`
	Type        FieldType `json:"type" yaml:"type"`
	Label       Label     `json:"label" yaml:"label"`
	Description Label     `json:"description,omitempty" yaml:"description"`
	Required    bool      `json:"required,omitempty" yaml:"required"`
	Unique      bool      `json:"unique,omitempty" yaml:"unique"`
	Searchable  bool      `json:"searchable,omitempty" yaml:"searchable"`
	// MaxLength limits text fields; 0 means unlimited.
	MaxLength int `json:"max_length,omitempty" yaml:"max_length"`
	// Min and Max bound number fields; nil means unbounded.
	Min *float64 `json:"min,omitempty" yaml:"min"`
	Max *float64 `json:"max,omitempty" yaml:"max"`
	// Options are the allowed values of a choice field.
	Options []Option `json:"options,omitempty" yaml:"options"`
	// To is the target of a reference field as "package.type".
	To string `json:"to,omitempty" yaml:"to"`
	// Default is the initial value, as the field's JSON representation.
	Default any `json:"default,omitempty" yaml:"default"`
	// Translatable text fields keep a value per language (PL21, E98): the column holds the
	// German value, the others live in the core table translation.
	Translatable bool `json:"translatable,omitempty" yaml:"translatable"`
	// Format "rich" makes a text field formatted text (UI7, Hauptplan E127): HTML the server
	// sanitizes on every write; the form shows the editor, lists and exports the plain text.
	Format string `json:"format,omitempty" yaml:"format"`
	// Formula makes the field calculated (PL22, E99): CEL over the other fields of the type,
	// computed on every write and stored; the field cannot be written through the API.
	Formula string `json:"formula,omitempty" yaml:"formula"`
	// Trait names the trait the field came from (PL17); set by the loader, empty for a
	// field the type declares itself.
	Trait string `json:"trait,omitempty" yaml:"-"`
	// Computed marks a field the core maintains (the sums of the trait costs, PL20; the
	// rollups, WF13): it is shown and filtered like any field but cannot be written through
	// the API.
	Computed bool `json:"computed,omitempty" yaml:"-"`
	// Hidden is set by the customer layer (PL43): the interface leaves the field out of forms
	// and lists, the API keeps it and its values.
	Hidden bool `json:"hidden,omitempty" yaml:"-"`
	// Rollup makes the field a sum or count over the records that point here (WF13).
	Rollup *Rollup `json:"rollup,omitempty" yaml:"rollup"`
}

// Rollup is a number the core keeps on a record from the records of another type that point
// to it through a reference field (WF13): the sum of one of their number fields, or how many
// they are, optionally only those a CEL condition holds for (it must translate to SQL like a
// permission condition). Records in the trash do not count.
type Rollup struct {
	// From is the type of the records counted, in the same package.
	From string `json:"from" yaml:"from"`
	// Via is the reference field of From that points to this type.
	Via string `json:"via" yaml:"via"`
	// Op is sum or count.
	Op string `json:"op" yaml:"op"`
	// Field is the number field of From that sum adds up.
	Field string `json:"field,omitempty" yaml:"field"`
	// Where narrows the records counted, CEL over `record`.
	Where string `json:"where,omitempty" yaml:"where"`
}

// State is one status of the state model of a type (WF1). Exactly one state is the initial
// one; a new record starts there.
type State struct {
	Name    string `json:"name" yaml:"name"`
	Label   Label  `json:"label" yaml:"label"`
	Initial bool   `json:"initial,omitempty" yaml:"initial"`
}

// Transition moves a record from one of the From states to the To state. When is a CEL
// condition over the record (Hauptplan E16); empty means always.
type Transition struct {
	Name  string   `json:"name" yaml:"name"`
	Label Label    `json:"label" yaml:"label"`
	From  []string `json:"from" yaml:"from"`
	To    string   `json:"to" yaml:"to"`
	When  string   `json:"when,omitempty" yaml:"when"`
}

// Type is an entity type: the definition behind one table (E33).
type Type struct {
	Name        string  `json:"name" yaml:"name"`
	Label       Label   `json:"label" yaml:"label"`
	Description Label   `json:"description,omitempty" yaml:"description"`
	Fields      []Field `json:"fields" yaml:"fields"`
	// TitleField names the field shown as the record's title in lists and links.
	TitleField string `json:"title_field,omitempty" yaml:"title_field"`
	// CountedBy names the rollup fields of other types that count this one (WF13), as
	// "package.type.field"; the loader fills it, so that a write knows without loading every
	// type whether it changes a sum.
	CountedBy []string `json:"counted_by,omitempty" yaml:"-"`
	// States and Transitions form the state model (WF1); a type without states has no status.
	States      []State      `json:"states,omitempty" yaml:"states"`
	Transitions []Transition `json:"transitions,omitempty" yaml:"transitions"`
	// Traits are the definition building blocks the type pulls in by name (architektur.md B2;
	// PL16 knows "list", PL17 adds the rest).
	Traits []string `json:"traits,omitempty" yaml:"traits"`
	// List holds the settings of a select list (trait "list", B3).
	List *ListSpec `json:"list,omitempty" yaml:"list"`
	// Actors are the roles at a record (trait "actors", WF3): requester, observer, assigned
	// and whatever the type needs, each with the types that may hold it.
	Actors []ActorRole `json:"actors,omitempty" yaml:"actors"`
	// FieldStates are the rules that make fields required, hidden, read-only or preset in some
	// states and form variants (WF2, B5).
	FieldStates []FieldStateRule `json:"field_states,omitempty" yaml:"field_states"`
	// Portal offers the type in the portal for users (UI10, Hauptplan E132): its records of
	// one's own, creating them with the fields named here, and answering in the timeline.
	Portal *Portal `json:"portal,omitempty" yaml:"portal"`
	// Package is set by the loader; it is not written in types/<name>.yaml.
	Package string `json:"package" yaml:"-"`
	// Revision is the hash of the effective definition (PL43): base and customer layer.
	Revision string `json:"revision,omitempty" yaml:"-"`
}

// HasStates reports whether the type carries a state model.
func (t Type) HasStates() bool { return len(t.States) > 0 }

// InitialState is the state of a new record; empty without a state model.
func (t Type) InitialState() string {
	for _, s := range t.States {
		if s.Initial {
			return s.Name
		}
	}
	return ""
}

// Transition returns the transition with that name, if any.
func (t Type) Transition(name string) (Transition, bool) {
	for _, tr := range t.Transitions {
		if tr.Name == name {
			return tr, true
		}
	}
	return Transition{}, false
}

// Allows reports whether the transition may start from the given state.
func (tr Transition) Allows(from string) bool {
	for _, f := range tr.From {
		if f == from {
			return true
		}
	}
	return false
}

// AllFields stands for every field in read_only of a field-state rule: the whole record.
const AllFields = "*"

// FieldStateRule is one rule of the field states (WF2, architektur.md B5, Inventar G53). It
// applies in the listed states (every state when empty) and, with When, only to records the
// CEL condition holds for: that is a form variant, such as "record.kind == 'fault'". Several
// rules that apply add up. Hidden is a matter of the form only; the API keeps such fields.
// Defaults preset fields of a new record that the caller leaves out.
type FieldStateRule struct {
	States   []string       `json:"states,omitempty" yaml:"states"`
	When     string         `json:"when,omitempty" yaml:"when"`
	Required []string       `json:"required,omitempty" yaml:"required"`
	Hidden   []string       `json:"hidden,omitempty" yaml:"hidden"`
	ReadOnly []string       `json:"read_only,omitempty" yaml:"read_only"`
	Defaults map[string]any `json:"defaults,omitempty" yaml:"defaults"`
}

// AppliesIn reports whether the rule covers a state.
func (r FieldStateRule) AppliesIn(state string) bool {
	if len(r.States) == 0 {
		return true
	}
	for _, s := range r.States {
		if s == state {
			return true
		}
	}
	return false
}

// ActorRole is one role at the records of a type (WF3, Inventar G37, G79): who may hold it
// (To: qualified type names, such as core.person, core.group or a supplier type) and whether
// it takes one holder only.
type ActorRole struct {
	Name   string   `json:"name" yaml:"name"`
	Label  Label    `json:"label" yaml:"label"`
	To     []string `json:"to" yaml:"to"`
	Single bool     `json:"single,omitempty" yaml:"single"`
}

// Actor returns the role with that name, if any.
func (t Type) Actor(name string) (ActorRole, bool) {
	for _, r := range t.Actors {
		if r.Name == name {
			return r, true
		}
	}
	return ActorRole{}, false
}

// Allows reports whether a record of the type may hold the role.
func (r ActorRole) Allows(typeName string) bool { return matchesType(r.To, typeName) }

// QualifiedName is "package.type", the name used in references.
func (t Type) QualifiedName() string { return t.Package + "." + t.Name }

// Field returns the field with that name, if any.
func (t Type) Field(name string) (Field, bool) {
	for _, f := range t.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// Package is the manifest plus everything the package ships.
type Package struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
	Label   Label  `json:"label" yaml:"label"`
	// Depends lists packages that must be installed first.
	Depends []string `json:"depends,omitempty" yaml:"depends"`
	// Extends names the package this one extends (layer "extension", E37); empty for a base package.
	Extends string `json:"extends,omitempty" yaml:"extends"`
	// Types in name order; set by the loader.
	Types []Type `json:"types" yaml:"-"`
	// Relations are the relation kinds below relations/ (PL15, KF2).
	Relations []RelationKind `json:"relations,omitempty" yaml:"-"`
	// Traits are the package's own traits below traits/ (PL17, B2): bundles of fields.
	Traits []Trait `json:"traits,omitempty" yaml:"-"`
}

// RelationKind is a kind of link between records (relations/<name>.yaml): what it is called
// in both directions, which types it links ("*" for any) and whether a problem at the target
// affects the source (impact analysis, E92).
type RelationKind struct {
	Package     string   `json:"package,omitempty" yaml:"-"`
	Name        string   `json:"name" yaml:"name"`
	Label       Label    `json:"label" yaml:"label"`
	Inverse     Label    `json:"inverse" yaml:"inverse"`
	Description Label    `json:"description,omitempty" yaml:"description"`
	From        []string `json:"from" yaml:"from"`
	To          []string `json:"to" yaml:"to"`
	Impact      bool     `json:"impact,omitempty" yaml:"impact"`
}

// QualifiedName is package.name.
func (k RelationKind) QualifiedName() string { return k.Package + "." + k.Name }

// Allows reports whether the kind links the two types.
func (k RelationKind) Allows(from, to string) bool {
	return matchesType(k.From, from) && matchesType(k.To, to)
}

func matchesType(allowed []string, typeName string) bool {
	for _, a := range allowed {
		if a == "*" || a == typeName {
			return true
		}
	}
	return false
}

// Type returns the type with that name, if any.
func (p Package) Type(name string) (Type, bool) {
	for _, t := range p.Types {
		if t.Name == name {
			return t, true
		}
	}
	return Type{}, false
}

func sortTypes(ts []Type) {
	sort.Slice(ts, func(i, j int) bool { return ts[i].Name < ts[j].Name })
}
