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
)

func customerBase() Type {
	return Type{Package: "p", Name: "t", TitleField: "title", Fields: []Field{
		{Name: "title", Type: FieldText, Label: Label{"de": "Titel", "en": "Title"}},
		{Name: "req", Type: FieldText, Required: true},
		{Name: "color", Type: FieldChoice, Label: Label{"de": "Farbe", "en": "Colour"}, Options: []Option{{Value: "red", Label: Label{"de": "Rot", "en": "Red"}}}},
	}}
}

func names(t Type) string {
	var out []string
	for _, f := range t.Fields {
		out = append(out, f.Name)
	}
	return strings.Join(out, ",")
}

// The changes apply in order; an inactive one is skipped; one that does not fit is a conflict
// and changes nothing; the base stays as it was.
func TestApplyCustomer(t *testing.T) {
	base := customerBase()
	yes := true
	l := CustomerLayer{Changes: []Change{
		{ID: "1", Op: OpAddField, Field: "k_a", New: &Field{Name: "k_a", Type: FieldText, Label: Label{"de": "A"}}},
		{ID: "2", Op: OpMoveField, Field: "k_a"},
		{ID: "3", Op: OpChangeField, Field: "title", Set: &FieldPatch{Label: Label{"de": "Betreff"}, Required: &yes}},
		{ID: "4", Op: OpAddOptions, Field: "color", Options: []Option{{Value: "green", Label: Label{"de": "Grün"}}}},
		{ID: "5", Op: OpHideField, Field: "color"},
		{ID: "6", Op: OpHideField, Field: "gone"},
		{ID: "7", Op: OpRemoveField, Field: "k_a", Inactive: "stilled"},
		{ID: "8", Op: OpChangeField, Field: "color", Set: &FieldPatch{Default: "green"}},
	}}
	got, conflicts := ApplyCustomer(base, l)
	if names(got) != "k_a,title,req,color" {
		t.Fatalf("fields: %s", names(got))
	}
	a, _ := got.Field("k_a")
	if a.Label["en"] != "A" {
		t.Fatalf("English from German: %v", a.Label)
	}
	title, _ := got.Field("title")
	if title.Label["de"] != "Betreff" || title.Label["en"] != "Title" || !title.Required {
		t.Fatalf("title: %+v", title)
	}
	color, _ := got.Field("color")
	if len(color.Options) != 2 || color.Options[1].Label["en"] != "Grün" || !color.Hidden || color.Default != "green" {
		t.Fatalf("color: %+v", color)
	}
	if len(conflicts) != 1 || conflicts[0].Change.ID != "6" || !strings.Contains(conflicts[0].Reason, "no field gone") {
		t.Fatalf("conflicts: %+v", conflicts)
	}
	if names(base) != "title,req,color" || len(base.Fields[2].Options) != 1 || base.Fields[0].Label["de"] != "Titel" {
		t.Fatalf("the base changed: %+v", base)
	}
}

// What the customer layer may not do to the fields of a package.
func TestApplyCustomerRefuses(t *testing.T) {
	five := 5
	for _, c := range []struct {
		change Change
		want   string
	}{
		{Change{Op: OpAddField, Field: "a", New: &Field{Name: "a", Type: FieldText}}, "k_"},
		{Change{Op: OpAddField, Field: "k_a"}, "needs the new field"},
		{Change{Op: OpRemoveField, Field: "color"}, "hide it instead"},
		{Change{Op: OpChangeField, Field: "color", Set: &FieldPatch{MaxLength: &five}}, "belongs to the package"},
		{Change{Op: OpChangeField, Field: "color", Set: &FieldPatch{Default: "blue"}}, "not one of the options"},
		{Change{Op: OpAddOptions, Field: "color", Options: []Option{{Value: "red"}}}, "already"},
		{Change{Op: OpAddOptions, Field: "title", Options: []Option{{Value: "x"}}}, "no choice"},
		{Change{Op: OpHideField, Field: "title"}, "stays visible"},
		{Change{Op: OpHideField, Field: "req"}, "nobody could create"},
		{Change{Op: OpMoveField, Field: "title", After: "nope"}, "no field nope"},
	} {
		_, conflicts := ApplyCustomer(customerBase(), CustomerLayer{Changes: []Change{c.change}})
		if len(conflicts) != 1 || !strings.Contains(conflicts[0].Reason, c.want) {
			t.Fatalf("%+v: want %q, got %+v", c.change, c.want, conflicts)
		}
	}
}

// A package may not name fields with the prefix of the customer layer.
func TestCheckPrefix(t *testing.T) {
	p := Package{Name: "p", Types: []Type{{Name: "t", Fields: []Field{{Name: "k_x", Type: FieldText}}}}}
	if err := checkPrefix(p); err == nil || !strings.Contains(err.Error(), "customer layer") {
		t.Fatalf("k_ in a package: %v", err)
	}
	p.Types[0].Fields[0].Name = "kx"
	if err := checkPrefix(p); err != nil {
		t.Fatal(err)
	}
}

// The state model in the customer layer (UI26.1): own states and transitions come and go,
// the initial mark moves, a transition of the package is disabled instead of removed; what
// the layer may not do is a conflict with a reason.
// TestApplyLayout: the layout of the graph (UI26.2) is one change with every state; a name
// that is no state is refused, a removed state leaves the layout, nil is refused, and the
// validation finds a stale name.
func TestApplyLayout(t *testing.T) {
	base := Type{Name: "thing", Package: "p", Label: Label{"de": "Ding", "en": "Thing"}, Fields: []Field{{Name: "title", Type: FieldText, Label: Label{"de": "Titel", "en": "Title"}}},
		States:      []State{{Name: "new", Label: Label{"de": "Neu", "en": "New"}, Initial: true}, {Name: "done", Label: Label{"de": "Erledigt", "en": "Done"}}},
		Transitions: []Transition{{Name: "finish", Label: Label{"de": "Erledigen", "en": "Finish"}, From: []string{"new"}, To: "done"}}}
	l := CustomerLayer{Changes: []Change{
		{ID: "1", Op: OpAddState, Field: "k_review", State: &State{Name: "k_review", Label: Label{"de": "Prüfung"}}},
		{ID: "2", Op: OpSetLayout, Field: "layout", Layout: map[string]Point{"new": {X: 0, Y: 0}, "done": {X: 240, Y: 0}, "k_review": {X: 120, Y: 140}}},
		{ID: "3", Op: OpSetLayout, Field: "layout", Layout: map[string]Point{"new": {X: 0, Y: 0}, "nowhere": {X: 1, Y: 1}}},
		{ID: "4", Op: OpRemoveState, Field: "k_review"},
		{ID: "5", Op: OpSetLayout, Field: "layout"},
	}}
	got, conflicts := ApplyCustomer(base, l)
	if len(got.Layout) != 2 || got.Layout["done"].X != 240 || got.Layout["new"] != (Point{}) {
		t.Fatalf("layout: %+v", got.Layout)
	}
	var ids []string
	for _, c := range conflicts {
		ids = append(ids, c.Change.ID+":"+c.Reason)
	}
	want := []string{"3:layout names nowhere, which is not a state of p.thing", "5:set_layout needs layout"}
	if strings.Join(ids, "|") != strings.Join(want, "|") {
		t.Fatalf("conflicts: %v", ids)
	}
	if base.Layout != nil {
		t.Fatalf("the base changed: %+v", base.Layout)
	}
	if err := ValidateCustomer(got, nil, nil, map[string]bool{}); err != nil {
		t.Fatalf("valid layout: %v", err)
	}
	got.Layout = map[string]Point{"gone": {}}
	if err := ValidateCustomer(got, nil, nil, map[string]bool{}); err == nil || !strings.Contains(err.Error(), `layout names "gone"`) {
		t.Fatalf("stale layout: %v", err)
	}
	// An empty layout takes the positions away.
	got, conflicts = ApplyCustomer(base, CustomerLayer{Changes: []Change{l.Changes[0], l.Changes[1], {ID: "6", Op: OpSetLayout, Field: "layout", Layout: map[string]Point{}}}})
	if len(conflicts) != 0 || got.Layout != nil {
		t.Fatalf("empty layout: %+v %+v", conflicts, got.Layout)
	}
}

func TestApplyWorkflow(t *testing.T) {
	base := Type{Name: "thing", Package: "p", Fields: []Field{{Name: "title", Type: FieldText, Label: Label{"de": "Titel", "en": "Title"}}},
		States:      []State{{Name: "new", Label: Label{"de": "Neu", "en": "New"}, Initial: true}, {Name: "done", Label: Label{"de": "Erledigt", "en": "Done"}}},
		Transitions: []Transition{{Name: "finish", Label: Label{"de": "Erledigen", "en": "Finish"}, From: []string{"new"}, To: "done"}}}
	yes, no, when := true, false, "record.title != \"\""
	l := CustomerLayer{Changes: []Change{
		{ID: "1", Op: OpAddState, Field: "k_review", State: &State{Name: "k_review", Label: Label{"de": "Prüfung"}}},
		{ID: "2", Op: OpAddTransition, Field: "k_review_it", Transition: &Transition{Name: "k_review_it", Label: Label{"de": "Prüfen"}, From: []string{"new"}, To: "k_review", When: "record.title == \"x\""}},
		{ID: "3", Op: OpChangeState, Field: "done", Set: &FieldPatch{Label: Label{"de": "Fertig"}}},
		{ID: "4", Op: OpChangeTransition, Field: "finish", Set: &FieldPatch{From: []string{"new", "k_review"}, When: &when}},
		{ID: "5", Op: OpDisableTransition, Field: "finish"},
		{ID: "6", Op: OpChangeState, Field: "k_review", Set: &FieldPatch{Initial: &yes}},
		// Refused: the package's transition cannot go, the package's state cannot go, the
		// initial state cannot lose its mark, a used state cannot go, names need k_.
		{ID: "7", Op: OpRemoveTransition, Field: "finish"},
		{ID: "8", Op: OpRemoveState, Field: "done"},
		{ID: "9", Op: OpChangeState, Field: "k_review", Set: &FieldPatch{Initial: &no}},
		{ID: "10", Op: OpRemoveState, Field: "k_review"},
		{ID: "11", Op: OpAddState, Field: "later", State: &State{Name: "later", Label: Label{"de": "Später"}}},
	}}
	got, conflicts := ApplyCustomer(base, l)
	if len(got.States) != 3 || got.States[2].Name != "k_review" || got.States[2].Label["en"] != "Prüfung" {
		t.Fatalf("states: %+v", got.States)
	}
	if !got.States[2].Initial || got.States[0].Initial || got.InitialState() != "k_review" {
		t.Fatalf("initial moved: %+v", got.States)
	}
	if got.States[1].Label["de"] != "Fertig" || got.States[1].Label["en"] != "Done" {
		t.Fatalf("relabel: %+v", got.States[1])
	}
	finish, _ := got.Transition("finish")
	if !finish.Disabled || finish.When != when || strings.Join(finish.From, ",") != "new,k_review" {
		t.Fatalf("finish: %+v", finish)
	}
	review, ok := got.Transition("k_review_it")
	if !ok || review.Label["en"] != "Prüfen" || review.To != "k_review" {
		t.Fatalf("k_review_it: %+v", review)
	}
	var ids []string
	for _, c := range conflicts {
		ids = append(ids, c.Change.ID+":"+c.Reason)
	}
	want := []string{"7:finish belongs to the package: disable it instead", "8:done belongs to the package and stays",
		"9:k_review is the initial state: mark another one initial instead", "10:k_review is the initial state: mark another one initial first",
		"11:a state of the customer layer is named k_…"}
	if strings.Join(ids, "|") != strings.Join(want, "|") {
		t.Fatalf("conflicts:\n%s\nwant\n%s", strings.Join(ids, "\n"), strings.Join(want, "\n"))
	}
	if len(base.States) != 2 || base.States[0].Initial != true || base.Transitions[0].Disabled || len(base.Transitions[0].From) != 1 {
		t.Fatalf("the base changed: %+v %+v", base.States, base.Transitions)
	}
	// Taking the initial mark away after it moved on: the used state cannot go while a
	// transition points at it.
	l2 := CustomerLayer{Changes: append(l.Changes[:6:6], Change{ID: "12", Op: OpChangeState, Field: "new", Set: &FieldPatch{Initial: &yes}}, Change{ID: "13", Op: OpRemoveState, Field: "k_review"})}
	_, conflicts = ApplyCustomer(base, l2)
	if len(conflicts) != 1 || conflicts[0].Change.ID != "13" || !strings.Contains(conflicts[0].Reason, "transition finish still uses k_review") {
		t.Fatalf("used state: %+v", conflicts)
	}
}
