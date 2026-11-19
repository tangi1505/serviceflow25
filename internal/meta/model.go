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
	"fmt"
	"regexp"
	"sort"
	"strings"
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
	// FieldSequence is a running number the core gives every new record, per type and tenant
	// (PL49, Kern-Plan E63): stored as a number, shown with the Prefix (INC-24817).
	FieldSequence FieldType = "sequence" // bigint
)

// Portal says how a type appears in the portal (UI10): the fields a user fills when creating
// and sees on the page of a record. With Read the type has an area of its own in the head of
// the portal where users browse and read what they may read (SD16a), such as articles.
type Portal struct {
	Fields []string    `json:"fields,omitempty" yaml:"fields"`
	Read   *PortalRead `json:"read,omitempty" yaml:"read"`
}

// PortalRead is the reading area of a type in the portal (SD16a): its name in the head, the
// fields its page shows under the title, and optionally a reference field to a tree list the
// area is browsed by.
type PortalRead struct {
	Label  Label    `json:"label" yaml:"label"`
	Fields []string `json:"fields" yaml:"fields"`
	Group  string   `json:"group,omitempty" yaml:"group"`
	// Facts are fields the list shows with each record, such as the delivery time and the
	// costs of an offer (SD13, SD-Plan N21); with facts the list shows cards.
	Facts []string `json:"facts,omitempty" yaml:"facts"`
	// Sort orders the list by a field ("-" before it: descending); empty: the newest first.
	Sort string `json:"sort,omitempty" yaml:"sort"`
	// Icon is the symbol of the area in the head of the portal (UI41, UI-Plan E43), one of
	// PackageIcons; empty: a book.
	Icon string `json:"icon,omitempty" yaml:"icon"`
}

// Reader is a rule by which signed-in persons read records of a type without a grant (SD16a,
// SD-Plan N12), as everyone reads the entries of the select lists (N5): the records the
// condition holds for, at the person's node or above. With Roles only the holders of these
// roles read them, wherever they are: a person named there or a member of a group named there.
// Actions may let them also comment (SD18, SD-Plan N15); reading is always among them.
type Reader struct {
	When    string   `json:"when,omitempty" yaml:"when"`
	Roles   []string `json:"roles,omitempty" yaml:"roles"`
	Actions []string `json:"actions,omitempty" yaml:"actions"`
}

// ReaderActions are what a reader rule may let do.
var ReaderActions = []string{"read", "comment"}

// Lets reports whether the rule lets do the action: read always, comment when named.
func (r Reader) Lets(action string) bool {
	if action == "read" {
		return true
	}
	for _, a := range r.Actions {
		if a == action {
			return true
		}
	}
	return false
}

// FormatRich marks a text field as formatted text (UI7).
const FormatRich = "rich"

// FormatPriority marks a number field as a priority (UI29): the interface shows it as a mark
// P1, P2, … instead of a plain number.
const FormatPriority = "priority"

// FormatIcon makes a text field a symbol of the fixed list PackageIcons (UI42, UI plan E53):
// the form offers the symbols as a grid to click, the server takes no other name.
const FormatIcon = "icon"

// FieldTypes lists the supported kinds in a stable order (for messages).
var FieldTypes = []FieldType{FieldText, FieldNumber, FieldBoolean, FieldDate, FieldDateTime, FieldChoice, FieldReference, FieldSequence}

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
	// Prefix comes before the number of a sequence field (PL49): "INC-" shows 24817 as
	// INC-24817. Changing it changes how every record shows.
	Prefix string `json:"prefix,omitempty" yaml:"prefix"`
	// Default is the initial value, as the field's JSON representation.
	Default any `json:"default,omitempty" yaml:"default"`
	// Translatable text fields keep a value per language (PL21, E98): the column holds the
	// German value, the others live in the core table translation.
	Translatable bool `json:"translatable,omitempty" yaml:"translatable"`
	// Format "rich" makes a text field formatted text (UI7, Hauptplan E127): HTML the server
	// sanitizes on every write; the form shows the editor, lists and exports the plain text.
	// "icon" makes it a symbol of PackageIcons (UI42); "priority" a number a priority (UI29).
	Format string `json:"format,omitempty" yaml:"format"`
	// Snippets names the use of the text modules the editor of a formatted text field offers
	// (SD8): "solution" for the solution, "task" for the text of a task; the entries of the
	// timeline take "answer".
	Snippets string `json:"snippets,omitempty" yaml:"snippets"`
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
// Point is a position in the workflow graph (UI26.2), in pixels of its canvas.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type State struct {
	Name    string `json:"name" yaml:"name"`
	Label   Label  `json:"label" yaml:"label"`
	Initial bool   `json:"initial,omitempty" yaml:"initial"`
	// Category says what kind of state it is, for the colour of its mark in the interface
	// (UI29): open, active, waiting or done. Empty: open for the initial state, else active.
	Category string `json:"category,omitempty" yaml:"category"`
}

// The categories of a state (UI29).
const (
	CategoryOpen    = "open"
	CategoryActive  = "active"
	CategoryWaiting = "waiting"
	CategoryDone    = "done"
)

// Transition moves a record from one of the From states to the To state. When is a CEL
// condition over the record (Hauptplan E16); empty means always.
type Transition struct {
	Name  string   `json:"name" yaml:"name"`
	Label Label    `json:"label" yaml:"label"`
	From  []string `json:"from" yaml:"from"`
	To    string   `json:"to" yaml:"to"`
	When  string   `json:"when,omitempty" yaml:"when"`
	// By names the roles at the record that alone may take the transition (SD-Plan N8): the
	// person or one of their groups holds one of them, such as the requester confirming a
	// solution in the portal. Taking it needs no other right on the type.
	By []string `json:"by,omitempty" yaml:"by"`
	// Comment "required" makes the transition take a text, kept as an entry at the record in
	// the same write (SD-Plan N8; the reason of a refused solution); "optional" offers it.
	Comment string `json:"comment,omitempty" yaml:"comment"`
	// Set gives fields fixed values in the same write (SD5): declaring a major incident sets
	// "major" to true. They win over the fields the caller sends.
	Set map[string]any `json:"set,omitempty" yaml:"set"`
	// Disabled is set by the customer layer (UI26.1): the transition stays in the definition
	// but is neither offered nor executed.
	Disabled bool `json:"disabled,omitempty" yaml:"-"`
}

// Transition comments (SD-Plan N8).
const (
	CommentRequired = "required"
	CommentOptional = "optional"
)

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
	// Layout is where the workflow editor draws each state in its graph (UI26.2, UI-Plan
	// E27): set by the customer layer, read by the interface alone, never by the engine. A
	// state without a position is arranged by the editor.
	Layout map[string]Point `json:"layout,omitempty" yaml:"-"`
	// Timed tells that SLA clocks run on the records of the type; the API sets it, so that the
	// list shows the deadline (UI34). No part of a package.
	Timed bool `json:"timed,omitempty" yaml:"-"`
	// May are the actions the session holds on the type somewhere: read, create, update,
	// delete, purge (PL75, Kern-Plan E79); the API sets it. No part of a package.
	May []string `json:"may,omitempty" yaml:"-"`
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
	// Readers let persons read records without a grant (SD16a): published articles for
	// everyone, for example. A select list reads like one reader without condition.
	Readers []Reader `json:"readers,omitempty" yaml:"readers"`
	// Suggest offers the type's records while records of other types are written (SD18):
	// articles at a ticket, for example.
	Suggest *Suggest `json:"suggest,omitempty" yaml:"suggest"`
	// Approval names the transitions that follow an approval (trait approvals, WF6).
	Approval *ApprovalSpec `json:"approval,omitempty" yaml:"approval"`
	// Order makes the records orderable with a form (SD13, SD-Plan N19): offers of the catalog.
	Order *OrderSpec `json:"order,omitempty" yaml:"order"`
	// Indexes are the indexes the package declares for the frequent lists of the type (PL54,
	// Kern-Plan E65), in addition to the one on created_at the core gives every type.
	Indexes []Index `json:"indexes,omitempty" yaml:"indexes"`
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

// LeadSuffix after a role names, as a recipient, the responsible persons of the groups that
// hold it: "actors.group.responsible", the lead of the team (SD-Plan N10).
const LeadSuffix = ".responsible"

// CheckRoleRecipient checks "<role>" or "<role>.responsible" of a recipient "actors.…": the
// role is one of the type, and one that groups may hold when it asks for their lead; "" if
// it fits.
func (t Type) CheckRoleRecipient(spec string) string {
	role, lead := strings.CutSuffix(spec, LeadSuffix)
	r, known := t.Actor(role)
	switch {
	case !known:
		return fmt.Sprintf("%q is not a role of %s", role, t.QualifiedName())
	case lead && !r.Allows("core.group"):
		return fmt.Sprintf("%q is no role of groups, so it has no responsible persons", role)
	}
	return ""
}

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
	// Icon is the symbol of the package's area in the navigation (UI36), one of PackageIcons;
	// empty gives the interface's own.
	Icon string `json:"icon,omitempty" yaml:"icon"`
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
	// Data is the start data below data/ (SD-Plan N9): records and rules the core creates once
	// per tenant.
	Data []Seed `json:"data,omitempty" yaml:"-"`
	// Settings are the settings of the organisation tree the package reads (SD20).
	Settings []Setting `json:"settings,omitempty" yaml:"settings"`
}

// RelationKind is a kind of link between records (relations/<name>.yaml): what it is called
// in both directions, which types it links ("*" for any) and whether a problem at the target
// affects the source (impact analysis, E92). Update says at which end linking needs the right
// to update: both (empty) or only from, where reading suffices at the target (SD18, SD-Plan
// N16): an agent links an article to an incident without the right to change the article.
type RelationKind struct {
	Package     string   `json:"package,omitempty" yaml:"-"`
	Name        string   `json:"name" yaml:"name"`
	Label       Label    `json:"label" yaml:"label"`
	Inverse     Label    `json:"inverse" yaml:"inverse"`
	Description Label    `json:"description,omitempty" yaml:"description"`
	From        []string `json:"from" yaml:"from"`
	To          []string `json:"to" yaml:"to"`
	Impact      bool     `json:"impact,omitempty" yaml:"impact"`
	Update      string   `json:"update,omitempty" yaml:"update"`
}

// RelationUpdateFrom is the value of Update for a kind that needs the right only at from.
const RelationUpdateFrom = "from"

// QualifiedName is package.name.
func (k RelationKind) QualifiedName() string { return k.Package + "." + k.Name }

// Allows reports whether the kind links records of the two types; an entry "trait:<p>.<t>"
// stands for every type with that trait (SD-Plan N11).
func (k RelationKind) Allows(from, to Type) bool {
	return relates(k.From, from) && relates(k.To, to)
}

// TraitPrefix marks an entry of a relation kind's from or to that names a trait (N11).
const TraitPrefix = "trait:"

func relates(allowed []string, t Type) bool {
	for _, a := range allowed {
		if trait, ok := strings.CutPrefix(a, TraitPrefix); ok {
			if t.HasTrait(trait) || (strings.HasPrefix(trait, t.Package+".") && t.HasTrait(strings.TrimPrefix(trait, t.Package+"."))) {
				return true
			}
			continue
		}
		if a == "*" || a == t.QualifiedName() {
			return true
		}
	}
	return false
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
