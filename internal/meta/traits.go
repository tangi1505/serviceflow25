// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"group24.de/itsm/internal/db/queries"
)

// --- traits (architektur.md B2; Hauptplan E94; PL17) ---
//
// A trait is a definition building block a type pulls in by name. The core ships the traits
// with behaviour: list (PL16), history, documents, notes, reservable, costs (PL20), actors
// (WF3), timeline (PL42), rating (SD18), approvals (WF6) and resolution (UI14a). A core trait may bring fields of its own (costs brings its sums). A package may add
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
	TraitRating     = "rating"     // helped or not, once per person (SD18)
	TraitApprovals  = "approvals"  // approval in stages, with the field approval (WF6)
	TraitResolution = "resolution" // when the record was resolved and how long it took (UI14a)
)

// The computed fields of the trait costs: the sums of the entries, kept by the core.
const (
	FieldTimeSpent = "time_spent" // minutes of all time entries
	FieldCostTotal = "cost_total" // amount of all entries
)

// StartsAtZero reports whether a computed field is a number the core keeps from zero: the sums
// of costs (PL20) and of rollups (WF13), never null. The durations of a resolution are empty
// until the record is resolved (UI14a).
func (f Field) StartsAtZero() bool {
	return f.Computed && f.Type == FieldNumber && f.Name != FieldResolutionHours && f.Name != FieldResolutionServiceHours
}

// The computed fields of the trait resolution (UI14a): set when the record reaches a state of
// the category done, emptied when it leaves it again.
const (
	FieldResolvedAt             = "resolved_at"              // the moment of the resolution
	FieldResolutionHours        = "resolution_hours"         // hours from creating to resolving, round the clock
	FieldResolutionServiceHours = "resolution_service_hours" // working hours by the calendar, without waiting
)

// Trait is one building block: its name, labels, the fields it gives a type and the
// sub-resources it opens below /api/v1/records/{type}/{id}/. Package is empty for the core.
type Trait struct {
	Package     string  `json:"package,omitempty" yaml:"-"`
	Name        string  `json:"name" yaml:"name"`
	Label       Label   `json:"label" yaml:"label"`
	Description Label   `json:"description,omitempty" yaml:"description"`
	Fields      []Field `json:"fields,omitempty" yaml:"fields"`
	// Actors are the roles a package trait gives a type besides its fields (SD-Plan N6): the
	// type gets them and the core trait actors with them.
	Actors       []ActorRole `json:"actors,omitempty" yaml:"actors"`
	SubResources []string    `json:"sub_resources,omitempty" yaml:"-"`
}

// CoreTraits are the traits with behaviour, in a stable order.
var CoreTraits = []Trait{
	{Name: TraitList, Label: Label{"de": "Auswahlliste", "en": "Select list"},
		Description: Label{"de": "Einträge in der Kerntabelle list_entry, Felder vom Merkmal, lesbar für jede angemeldete Person an ihrem Knoten und darunter", "en": "Entries in the core table list_entry, fields from the trait, readable by every signed-in person at their node and below"}},
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
	{Name: TraitRating, Label: Label{"de": "Bewertung", "en": "Rating"}, SubResources: []string{"rating"},
		Description: Label{"de": "Jede Person sagt einmal, ob der Datensatz geholfen hat, und kann es ändern", "en": "Every person says once whether the record helped, and may change it"}},
	{Name: TraitApprovals, Label: Label{"de": "Genehmigungen", "en": "Approvals"}, SubResources: []string{"approvals"},
		Description: Label{"de": "Genehmigung in Stufen nacheinander, in einer Stufe mit Mindestzahl; das Feld Genehmigung zeigt den Stand", "en": "Approval in stages one after the other, each with a least number of yes; the field approval shows the outcome"},
		Fields:      []Field{approvalField}},
	{Name: TraitResolution, Label: Label{"de": "Lösung", "en": "Resolution"},
		Description: Label{"de": "Hält fest, wann der Datensatz einen erledigten Status erreicht hat, und die Dauer seit dem Anlegen in Kalenderzeit und in Servicezeit ohne Wartezeit", "en": "Keeps when the record reached a done state, and the time since its creation round the clock and in working time without waiting"},
		Fields: []Field{
			{Name: FieldResolvedAt, Type: FieldDateTime, Computed: true,
				Label: Label{"de": "Gelöst am", "en": "Resolved at"}},
			{Name: FieldResolutionHours, Type: FieldNumber, Computed: true,
				Label: Label{"de": "Lösungsdauer (Stunden)", "en": "Time to resolve (hours)"}},
			{Name: FieldResolutionServiceHours, Type: FieldNumber, Computed: true,
				Label: Label{"de": "Lösungsdauer in Servicezeit (Stunden)", "en": "Time to resolve in service time (hours)"}},
		}},
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
// (costs). A package trait may bring roles too; the type then has the core trait actors. The
// list trait has expandList.
func expandTraits(t *Type, p Package) error {
	done := map[string]bool{}
	withActors := false
	for _, name := range t.Traits {
		if strings.Contains(name, ".") {
			// The trait of another package (SD-Plan N7): ResolveTraits, once the installed
			// packages are known.
			continue
		}
		tr, ok := p.Trait(name)
		if core, isCore := CoreTrait(name); isCore && name != TraitList {
			tr, ok = core, len(core.Fields) > 0
		}
		if !ok || done[name] {
			// Unknown and repeated names are reported by Validate.
			continue
		}
		done[name] = true
		if err := addTrait(t, name, tr); err != nil {
			return err
		}
		withActors = withActors || len(tr.Actors) > 0
	}
	if withActors && !t.HasTrait(TraitActors) {
		t.Traits = append(t.Traits, TraitActors)
	}
	return nil
}

// addTrait gives a type the fields and roles of one trait, each field marked with the trait.
func addTrait(t *Type, name string, tr Trait) error {
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
	for _, r := range tr.Actors {
		if _, dup := t.Actor(r.Name); dup {
			return fmt.Errorf("role %q of trait %q collides with a role of the type", r.Name, name)
		}
		t.Actors = append(t.Actors, r)
	}
	return nil
}

// ResolveTraits gives the types of a package the traits they name with the name of another
// package, such as traits: [sd.ticket] (SD-Plan N7), from the traits installed. The package
// must depend on that package. Load leaves these traits for this step, which needs the
// installed packages; call it once, before Store and schema.Apply.
func ResolveTraits(p *Package, installed []Trait) error {
	byName := map[string]Trait{}
	for _, tr := range installed {
		if tr.Package != "" {
			byName[tr.Package+"."+tr.Name] = tr
		}
	}
	for i := range p.Types {
		t := &p.Types[i]
		withActors := false
		for _, name := range t.Traits {
			pkg, _, foreign := strings.Cut(name, ".")
			if !foreign {
				continue
			}
			if !slices.Contains(p.Depends, pkg) {
				return fmt.Errorf("types/%s.yaml: trait %q needs depends: [%s] in package.yaml", t.Name, name, pkg)
			}
			tr, ok := byName[name]
			if !ok {
				return fmt.Errorf("types/%s.yaml: trait %q is not installed; install package %s first", t.Name, name, pkg)
			}
			if err := addTrait(t, name, tr); err != nil {
				return fmt.Errorf("types/%s.yaml: %w", t.Name, err)
			}
			withActors = withActors || len(tr.Actors) > 0
		}
		if withActors && !t.HasTrait(TraitActors) {
			t.Traits = append(t.Traits, TraitActors)
		}
	}
	return nil
}

// ResolveInstalled is ResolveTraits with the traits installed in the database.
func ResolveInstalled(ctx context.Context, q *queries.Queries, p *Package) error {
	installed, err := Traits(ctx, q)
	if err != nil {
		return err
	}
	return ResolveTraits(p, installed)
}

// validateTraits checks the trait names of a type: known (core or this package), no repeats.
func validateTraits(add func(string, string, ...any), where string, t Type, p Package) {
	seen := map[string]bool{}
	for _, name := range t.Traits {
		_, core := CoreTrait(name)
		_, own := p.Trait(name)
		// The trait of another package counts once ResolveTraits gave its fields (N7).
		foreign := strings.Contains(name, ".") && slices.ContainsFunc(t.Fields, func(f Field) bool { return f.Trait == name })
		if !core && !own && !foreign {
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
