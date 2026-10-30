// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"strings"
	"testing"
	"testing/fstest"
)

const taggedTrait = `name: tagged
label: {de: Gekennzeichnet, en: Tagged}
fields:
  - {name: tag_no, type: text, label: {de: N, en: N}, max_length: 50}
  - {name: tagged_on, type: date, label: {de: D, en: D}}
`

// A type pulls traits in by name (PL17): core traits give behaviour, a package trait gives
// its fields, each marked with the trait; a type without history has none.
func TestTraitsExpandFieldsAndMarkThem(t *testing.T) {
	p, err := LoadFS(fstest.MapFS{
		"package.yaml":       {Data: []byte(manifest)},
		"traits/tagged.yaml": {Data: []byte(taggedTrait)},
		"types/thing.yaml":   {Data: []byte("name: thing\nlabel: {de: D, en: T}\ntraits: [history, documents, tagged]\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
		"types/plain.yaml":   {Data: []byte("name: plain\nlabel: {de: P, en: P}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
		"types/tag.yaml":     {Data: []byte("name: tag\nlabel: {de: Tag, en: Tag}\ntraits: [list]\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(p, nil); err != nil {
		t.Fatal(err)
	}
	if len(p.Traits) != 1 || p.Traits[0].Name != "tagged" || p.Traits[0].Package != "metatest" {
		t.Fatalf("traits: %+v", p.Traits)
	}
	plain, tag, thing := p.Types[0], p.Types[1], p.Types[2]
	if !thing.HasHistory() || !thing.HasTrait(TraitDocuments) || len(thing.Fields) != 3 {
		t.Fatalf("thing: %+v", thing)
	}
	if f, _ := thing.Field("tag_no"); f.Trait != "tagged" || f.MaxLength != 50 {
		t.Fatalf("tag_no: %+v", f)
	}
	if f, _ := thing.Field("title"); f.Trait != "" {
		t.Fatalf("title came from a trait: %+v", f)
	}
	if subs := strings.Join(thing.SubResources(), ","); subs != "history,documents" {
		t.Fatalf("sub-resources: %s", subs)
	}
	if plain.HasHistory() || len(plain.SubResources()) != 0 {
		t.Fatalf("plain: %+v", plain)
	}
	// A list always has history, and its fields carry the list trait.
	if !tag.HasHistory() || tag.Fields[0].Trait != TraitList {
		t.Fatalf("tag: %+v", tag)
	}
}

func TestTraitsRejectUnknownCollidingAndBad(t *testing.T) {
	_, err := LoadFS(fstest.MapFS{
		"package.yaml":       {Data: []byte(manifest)},
		"traits/tagged.yaml": {Data: []byte(taggedTrait)},
		"types/thing.yaml":   {Data: []byte("name: thing\nlabel: {de: D, en: T}\ntraits: [tagged]\nfields:\n  - {name: tag_no, type: number, label: {de: T, en: T}}\n")},
	})
	if err == nil || !strings.Contains(err.Error(), `field "tag_no" of trait "tagged" collides`) {
		t.Fatalf("collision: %v", err)
	}
	_, err = LoadFS(fstest.MapFS{
		"package.yaml":       {Data: []byte(manifest)},
		"traits/tagged.yaml": {Data: []byte(taggedTrait)},
		"types/tag.yaml":     {Data: []byte("name: tag\nlabel: {de: Tag, en: Tag}\ntraits: [list, tagged]\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "a list takes its fields from its trait") {
		t.Fatalf("list with a package trait: %v", err)
	}

	p, err := LoadFS(fstest.MapFS{
		"package.yaml":        {Data: []byte(manifest)},
		"traits/history.yaml": {Data: []byte("name: history\nlabel: {de: H, en: H}\nfields:\n  - {name: id, type: text, label: {de: I, en: I}}\n  - {name: x, type: nope, label: {de: X, en: X}}\n")},
		"traits/empty.yaml":   {Data: []byte("name: empty\nlabel: {de: E, en: E}\n")},
		"types/thing.yaml":    {Data: []byte("name: thing\nlabel: {de: D, en: T}\ntraits: [shiny, history, history]\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(problems(t, Validate(p, nil)), "\n")
	for _, want := range []string{
		`trait history: name "history" is taken by a core trait`,
		`trait history, field id: name "id" is reserved for a system column`,
		`trait history, field x: unknown field type "nope"`,
		`trait empty: has no fields`,
		`type thing: unknown trait "shiny"`,
		`type thing: trait "history" listed twice`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// The core trait costs brings its sums as computed fields (PL20); a list cannot take it, and
// a type with a field of the same name collides.
func TestCostsTraitBringsComputedSums(t *testing.T) {
	p, err := LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte(manifest)},
		"types/thing.yaml": {Data: []byte("name: thing\nlabel: {de: D, en: T}\ntraits: [history, costs]\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(p, nil); err != nil {
		t.Fatal(err)
	}
	thing := p.Types[0]
	for _, name := range []string{FieldTimeSpent, FieldCostTotal} {
		if f, ok := thing.Field(name); !ok || !f.Computed || f.Trait != TraitCosts || f.Type != FieldNumber {
			t.Fatalf("%s: %+v", name, f)
		}
	}
	if subs := strings.Join(thing.SubResources(), ","); subs != "history,costs" {
		t.Fatalf("sub-resources: %s", subs)
	}
	for name, yaml := range map[string]string{
		"list":      "name: x\nlabel: {de: Tag, en: Tag}\ntraits: [list, costs]\n",
		"collision": "name: x\nlabel: {de: D, en: T}\ntraits: [costs]\nfields:\n  - {name: cost_total, type: number, label: {de: K, en: C}}\n",
	} {
		if _, err := LoadFS(fstest.MapFS{"package.yaml": {Data: []byte(manifest)}, "types/x.yaml": {Data: []byte(yaml)}}); err == nil || !strings.Contains(err.Error(), "costs") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Only text fields can be translatable (PL21); the name and description of a select list are.
func TestTranslatableFields(t *testing.T) {
	p, err := LoadFS(fstest.MapFS{
		"package.yaml":   {Data: []byte(manifest)},
		"types/bad.yaml": {Data: []byte("name: bad\nlabel: {de: B, en: B}\nfields:\n  - {name: amount, type: number, label: {de: A, en: A}, translatable: true}\n")},
		"types/tag.yaml": {Data: []byte("name: tag\nlabel: {de: Tag, en: Tag}\ntraits: [list]\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(p, nil); err == nil || !strings.Contains(err.Error(), "translatable only applies to text fields") {
		t.Fatalf("translatable number: %v", err)
	}
	tag := p.Types[1]
	for _, name := range []string{"name", "description"} {
		if f, _ := tag.Field(name); !f.Translatable {
			t.Errorf("list field %s is not translatable", name)
		}
	}
	if f, _ := tag.Field("key"); f.Translatable {
		t.Error("the key of a list is translatable")
	}
}

// Formulas are checked when the package is installed (PL22): type errors, references,
// cycles, calculated fields that are required, and the sums of the costs.
func TestFormulasAreCheckedAtInstall(t *testing.T) {
	load := func(fields string) error {
		p, err := LoadFS(fstest.MapFS{
			"package.yaml":     {Data: []byte(manifest)},
			"types/thing.yaml": {Data: []byte("name: thing\nlabel: {de: D, en: T}\ntraits: [costs]\nfields:\n  - {name: impact, type: number, label: {de: I, en: I}}\n  - {name: urgency, type: number, label: {de: U, en: U}}\n" + fields)},
		})
		if err != nil {
			return err
		}
		return Validate(p, nil)
	}
	if err := load("  - {name: priority, type: number, label: {de: P, en: P}, formula: 'impact * urgency'}\n  - {name: band, type: text, label: {de: B, en: B}, formula: 'priority > 4.0 ? \"high\" : \"low\"'}\n"); err != nil {
		t.Fatalf("valid formulas: %v", err)
	}
	for name, c := range map[string]struct{ fields, want string }{
		"typo":      {"  - {name: priority, type: number, label: {de: P, en: P}, formula: 'impact * urgncy'}\n", "undeclared reference to 'urgncy'"},
		"result":    {"  - {name: priority, type: text, label: {de: P, en: P}, formula: 'impact * urgency'}\n", "the field needs string"},
		"cycle":     {"  - {name: a, type: number, label: {de: A, en: A}, formula: 'b + 1.0'}\n  - {name: b, type: number, label: {de: B, en: B}, formula: 'a + 1.0'}\n", "cycle: a → b → a"},
		"self":      {"  - {name: a, type: number, label: {de: A, en: A}, formula: 'a + 1.0'}\n", "undeclared reference to 'a'"},
		"required":  {"  - {name: a, type: number, label: {de: A, en: A}, required: true, formula: 'impact'}\n", "cannot be required"},
		"reference": {"  - {name: a, type: reference, to: metatest.thing, label: {de: A, en: A}, formula: '\"x\"'}\n", "a reference cannot be calculated"},
		"sums":      {"  - {name: a, type: number, label: {de: A, en: A}, formula: 'cost_total * 2.0'}\n", "undeclared reference to 'cost_total'"},
	} {
		if err := load(c.fields); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
	// The order puts a formula after the calculated fields it uses.
	p, _ := LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte(manifest)},
		"types/thing.yaml": {Data: []byte("name: thing\nlabel: {de: D, en: T}\nfields:\n  - {name: x, type: number, label: {de: X, en: X}}\n  - {name: a, type: number, label: {de: A, en: A}, formula: 'b * 2.0'}\n  - {name: b, type: number, label: {de: B, en: B}, formula: 'x + 1.0'}\n")},
	})
	cs, err := CompileFormulas(p.Types[0])
	if err != nil || len(cs) != 2 || cs[0].Field.Name != "b" || cs[1].Field.Name != "a" {
		t.Fatalf("order: %+v %v", cs, err)
	}
	if f, _ := p.Types[0].Field("a"); !f.Calculated() {
		t.Fatal("a formula field is calculated")
	}
}
