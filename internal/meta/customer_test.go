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
