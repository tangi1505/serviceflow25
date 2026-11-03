// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

// TraitList marks a type as a select list (architektur.md B3; PL16; Hauptplan E93): its
// entries live in the core table list_entry instead of a table of their own, its fields come
// from the trait (ListFields), and every signed-in person may read it.
const TraitList = "list"

// ListSpec are the settings of a select list: a tree has a parent per entry and a full name
// "Country > City"; coordinates add lat, lon and alt. Entries are the values the package
// ships; they are created on install when their key is missing and never changed afterwards
// (the data belongs to the customer).
type ListSpec struct {
	Tree        bool        `json:"tree,omitempty" yaml:"tree"`
	Coordinates bool        `json:"coordinates,omitempty" yaml:"coordinates"`
	Entries     []ListEntry `json:"entries,omitempty" yaml:"entries"`
}

// ListEntry is one shipped value of a select list. Parent names the key of an earlier entry.
type ListEntry struct {
	Key  string `json:"key" yaml:"key"`
	Name string `json:"name" yaml:"name"`
	// NameEn is the English name, kept as the translation of the name (PL21; SD1).
	NameEn      string   `json:"name_en,omitempty" yaml:"name_en"`
	Description string   `json:"description,omitempty" yaml:"description"`
	Parent      string   `json:"parent,omitempty" yaml:"parent"`
	Position    *float64 `json:"position,omitempty" yaml:"position"`
	Lat         *float64 `json:"lat,omitempty" yaml:"lat"`
	Lon         *float64 `json:"lon,omitempty" yaml:"lon"`
	Alt         *float64 `json:"alt,omitempty" yaml:"alt"`
}

// HasTrait reports whether the type carries the trait.
func (t Type) HasTrait(name string) bool {
	for _, tr := range t.Traits {
		if tr == name {
			return true
		}
	}
	return false
}

// IsList reports whether the type is a select list.
func (t Type) IsList() bool { return t.HasTrait(TraitList) }

// IsTree reports whether the type is a select list arranged as a tree.
func (t Type) IsTree() bool { return t.IsList() && t.List != nil && t.List.Tree }

// ListFields are the fields of a select list, in output order: name, key, description,
// active, position; parent for a tree; lat, lon, alt with coordinates. self is the qualified
// name of the list, the target of parent.
func ListFields(spec ListSpec, self string) []Field {
	fields := []Field{
		{Name: "name", Type: FieldText, Label: Label{"de": "Name", "en": "Name"}, Required: true, Searchable: true, MaxLength: 200, Translatable: true},
		{Name: "key", Type: FieldText, Label: Label{"de": "Schlüssel", "en": "Key"}, Unique: true, MaxLength: 100,
			Description: Label{"de": "Fester Name für Regeln und Pakete, eindeutig je Liste", "en": "Stable name for rules and packages, unique per list"}},
		{Name: "description", Type: FieldText, Label: Label{"de": "Beschreibung", "en": "Description"}, MaxLength: 2000, Translatable: true},
		{Name: "active", Type: FieldBoolean, Label: Label{"de": "Aktiv", "en": "Active"}, Default: true},
		{Name: "position", Type: FieldNumber, Label: Label{"de": "Reihenfolge", "en": "Position"}},
	}
	if spec.Tree {
		fields = append(fields, Field{Name: "parent", Type: FieldReference, Label: Label{"de": "Übergeordnet", "en": "Parent"}, To: self})
	}
	if spec.Coordinates {
		latMin, latMax, lonMin, lonMax := -90.0, 90.0, -180.0, 180.0
		fields = append(fields,
			Field{Name: "lat", Type: FieldNumber, Label: Label{"de": "Breitengrad", "en": "Latitude"}, Min: &latMin, Max: &latMax},
			Field{Name: "lon", Type: FieldNumber, Label: Label{"de": "Längengrad", "en": "Longitude"}, Min: &lonMin, Max: &lonMax},
			Field{Name: "alt", Type: FieldNumber, Label: Label{"de": "Höhe", "en": "Altitude"}},
		)
	}
	return fields
}

// expandList gives a list type its fields and title field. It is called by the loader; a
// list type in types/<name>.yaml declares no fields of its own.
func expandList(t *Type, pkg string) {
	if !t.IsList() {
		return
	}
	if t.List == nil {
		t.List = &ListSpec{}
	}
	t.Fields = ListFields(*t.List, pkg+"."+t.Name)
	for i := range t.Fields {
		t.Fields[i].Trait = TraitList
	}
	t.TitleField = "name"
	// Who changed a list is always worth knowing.
	if !t.HasHistory() {
		t.Traits = append(t.Traits, TraitHistory)
	}
}

// validateList checks the list settings and the shipped entries of a type.
func validateList(add func(string, string, ...any), where string, t Type) {
	if !t.IsList() {
		if t.List != nil {
			add(where, "\"list\" settings need traits: [list]")
		}
		return
	}
	if t.HasStates() {
		add(where, "a list has no state model")
	}
	if t.List == nil {
		return
	}
	keys := map[string]bool{}
	for i, e := range t.List.Entries {
		ew := where + ", entry " + e.Key
		if !IsIdent(e.Key) {
			add(where, "entry %d: key %q must be snake_case (a-z, 0-9, _), at most 63 characters", i+1, e.Key)
		}
		if keys[e.Key] {
			add(ew, "key listed twice")
		}
		if e.Name == "" {
			add(ew, "needs a name")
		}
		if e.Parent != "" {
			if !t.List.Tree {
				add(ew, "parent needs tree: true")
			} else if !keys[e.Parent] {
				add(ew, "parent %q must be the key of an earlier entry", e.Parent)
			}
		}
		if !t.List.Coordinates && (e.Lat != nil || e.Lon != nil || e.Alt != nil) {
			add(ew, "lat, lon and alt need coordinates: true")
		}
		keys[e.Key] = true
	}
}
