// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import "fmt"

// --- traits (architektur.md B2; Hauptplan E94; PL17) ---
//
// A trait is a definition building block a type pulls in by name. The core ships the traits
// with behaviour: list (PL16), history, documents, notes, reservable, costs (PL20), actors
// (WF3) and timeline (PL42). A core trait may bring fields of its own (costs brings its sums). A package may add
// traits of its own below traits/<name>.yaml: bundles of fields every type of the package
// can take, such as finance data. A type names its traits in "traits: [...]".

// Core trait names.
const (
	TraitHistory    = "history"    // audit rows per change and GET …/history
	TraitDocuments  = "documents"  // files attached to the record (PL18)
	TraitNotes      = "notes"      // notes on the record (PL35)
	TraitReservable = "reservable" // reservations of the record
	TraitCosts      = "costs"      // cost and time entries with sums on the record (PL20)
	TraitActors     = "actors"     // roles at the record: requester, observer, assigned, … (WF3)
	TraitTimeline   = "timeline"   // entries public or private, with the history in order (PL42)
)

// The computed fields of the trait costs: the sums of the entries, kept by the core.
const (
	FieldTimeSpent = "time_spent" // minutes of all time entries
	FieldCostTotal = "cost_total" // amount of all entries
)

// Trait is one building block: its name, labels, the fields it gives a type and the
// sub-resources it opens below /api/v1/records/{type}/{id}/. Package is empty for the core.
type Trait struct {
	Package      string   `json:"package,omitempty" yaml:"-"`
	Name         string   `json:"name" yaml:"name"`
	Label        Label    `json:"label" yaml:"label"`
	Description  Label    `json:"description,omitempty" yaml:"description"`
	Fields       []Field  `json:"fields,omitempty" yaml:"fields"`
	SubResources []string `json:"sub_resources,omitempty" yaml:"-"`
}

// CoreTraits are the traits with behaviour, in a stable order.
var CoreTraits = []Trait{
	{Name: TraitList, Label: Label{"de": "Auswahlliste", "en": "Select list"},
		Description: Label{"de": "Einträge in der Kerntabelle list_entry, Felder vom Merkmal, lesbar für jede angemeldete Person", "en": "Entries in the core table list_entry, fields from the trait, readable by every signed-in person"}},
	{Name: TraitHistory, Label: Label{"de": "Verlauf", "en": "History"}, SubResources: []string{"history"},
		Description: Label{"de": "Jede Änderung hinterlässt Verlaufszeilen je Feld", "en": "Every change leaves audit rows per field"}},
	{Name: TraitDocuments, Label: Label{"de": "Dokumente", "en": "Documents"}, SubResources: []string{"documents"},
		Description: Label{"de": "Dateien am Datensatz", "en": "Files attached to the record"}},
	{Name: TraitNotes, Label: Label{"de": "Notizen", "en": "Notes"}, SubResources: []string{"notes"},
		Description: Label{"de": "Notizen am Datensatz", "en": "Notes on the record"}},
	{Name: TraitReservable, Label: Label{"de": "Reservierbar", "en": "Reservable"}, SubResources: []string{"reservations"},
		Description: Label{"de": "Der Datensatz kann reserviert werden", "en": "The record can be reserved"}},
	{Name: TraitCosts, Label: Label{"de": "Kosten und Zeiten", "en": "Costs and times"}, SubResources: []string{"costs"},
		Description: Label{"de": "Zeit-, Fest- und Materialkosten am Datensatz, mit Summen am Datensatz", "en": "Time, fixed and material costs on the record, with sums on the record"},
		Fields: []Field{
			{Name: FieldTimeSpent, Type: FieldNumber, Computed: true,
				Label: Label{"de": "Aufgewendete Zeit (Minuten)", "en": "Time spent (minutes)"}},
			{Name: FieldCostTotal, Type: FieldNumber, Computed: true,
				Label: Label{"de": "Kosten gesamt", "en": "Total cost"}},
		}},
	{Name: TraitActors, Label: Label{"de": "Rollen am Datensatz", "en": "Roles at the record"},
		Description: Label{"de": "Personen, Gruppen oder Lieferanten in den Rollen, die der Typ unter actors nennt", "en": "Persons, groups or suppliers in the roles the type names under actors"}},
	{Name: TraitTimeline, Label: Label{"de": "Zeitleiste", "en": "Timeline"}, SubResources: []string{"timeline"},
		Description: Label{"de": "Einträge öffentlich oder privat, mit dem Verlauf in zeitlicher Reihenfolge", "en": "Entries public or private, with the history in order of time"}},
}

// CoreTrait returns the core trait with that name.
func CoreTrait(name string) (Trait, bool) {
	for _, tr := range CoreTraits {
		if tr.Name == name {
			return tr, true
		}
	}
	return Trait{}, false
}

// Trait returns the package's own trait with that name.
func (p Package) Trait(name string) (Trait, bool) {
	for _, tr := range p.Traits {
		if tr.Name == name {
			return tr, true
		}
	}
	return Trait{}, false
}

// HasHistory reports whether changes of the type leave audit rows.
func (t Type) HasHistory() bool { return t.HasTrait(TraitHistory) }

// SubResources lists the sub-resources the type's core traits open, in trait order.
func (t Type) SubResources() []string {
	var out []string
	for _, tr := range CoreTraits {
		if t.HasTrait(tr.Name) {
			out = append(out, tr.SubResources...)
		}
	}
	return out
}

// expandTraits appends the fields of the traits to the type, each marked with the trait it
// came from: the fields of the package's own traits and of core traits that bring fields
// (costs). The list trait has expandList.
func expandTraits(t *Type, p Package) error {
	done := map[string]bool{}
	for _, name := range t.Traits {
		tr, ok := p.Trait(name)
		if core, isCore := CoreTrait(name); isCore && name != TraitList {
			tr, ok = core, len(core.Fields) > 0
		}
		if !ok || done[name] {
			// Unknown and repeated names are reported by Validate.
			continue
		}
		done[name] = true
		if t.IsList() {
			return fmt.Errorf("a list takes its fields from its trait; it cannot take trait %q", name)
		}
		for _, f := range tr.Fields {
			if _, dup := t.Field(f.Name); dup {
				return fmt.Errorf("field %q of trait %q collides with a field of the type", f.Name, name)
			}
			f.Trait = name
			t.Fields = append(t.Fields, f)
		}
	}
	return nil
}

// validateTraits checks the trait names of a type: known (core or this package), no repeats.
func validateTraits(add func(string, string, ...any), where string, t Type, p Package) {
	seen := map[string]bool{}
	for _, name := range t.Traits {
		_, core := CoreTrait(name)
		_, own := p.Trait(name)
		if !core && !own {
			add(where, "unknown trait %q; the core knows %s, this package adds %s", name, traitNames(CoreTraits), traitNames(p.Traits))
		}
		if seen[name] {
			add(where, "trait %q listed twice", name)
		}
		seen[name] = true
	}
}

func traitNames(ts []Trait) string {
	if len(ts) == 0 {
		return "none"
	}
	s := ""
	for i, tr := range ts {
		if i > 0 {
			s += ", "
		}
		s += tr.Name
	}
	return s
}
