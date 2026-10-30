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

// A list type gets its fields from the trait (PL16): name, key, description, active,
// position, plus parent for a tree and lat, lon, alt with coordinates; the title is name.
func TestListTypeTakesFieldsFromTrait(t *testing.T) {
	p := pkg(t, map[string]string{
		"tag": "name: tag\nlabel: {de: Tag, en: Tag}\ntraits: [list]\nlist:\n  entries:\n    - {key: a, name: A}\n",
		"region": `name: region
label: {de: Region, en: Region}
traits: [list]
list:
  tree: true
  coordinates: true
  entries:
    - {key: eu, name: Europa}
    - {key: de, name: Deutschland, parent: eu, lat: 51, lon: 10}
`,
	})
	if err := Validate(p, nil); err != nil {
		t.Fatal(err)
	}
	tag, region := p.Types[1], p.Types[0]
	if tag.Name != "tag" || !tag.IsList() || tag.IsTree() || tag.TitleField != "name" {
		t.Fatalf("tag: %+v", tag)
	}
	names := func(ty Type) string {
		var out []string
		for _, f := range ty.Fields {
			out = append(out, f.Name)
		}
		return strings.Join(out, ",")
	}
	if names(tag) != "name,key,description,active,position" {
		t.Fatalf("tag fields: %s", names(tag))
	}
	if !region.IsTree() || names(region) != "name,key,description,active,position,parent,lat,lon,alt" {
		t.Fatalf("region fields: %s", names(region))
	}
	if f, _ := region.Field("parent"); f.To != "metatest.region" {
		t.Fatalf("parent points to %q", f.To)
	}
	if f, _ := region.Field("key"); !f.Unique {
		t.Fatal("key must be unique")
	}
}

func TestListTypeRejectsOwnFieldsAndBadSettings(t *testing.T) {
	_, err := LoadFS(fstest.MapFS{
		"package.yaml":   {Data: []byte(manifest)},
		"types/tag.yaml": {Data: []byte("name: tag\nlabel: {de: T, en: T}\ntraits: [list]\nfields:\n  - {name: x, type: text, label: {de: X, en: X}}\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "takes its fields from the trait") {
		t.Fatalf("own fields: %v", err)
	}

	p := pkg(t, map[string]string{
		"plain": "name: plain\nlabel: {de: P, en: P}\nlist: {tree: true}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n",
		"odd":   "name: odd\nlabel: {de: O, en: O}\ntraits: [list, shiny, list]\nlist:\n  entries:\n    - {key: Bad-Key, name: X}\n    - {key: b, name: ''}\n    - {key: b, name: B, parent: a, lat: 1}\n",
		"tree":  "name: tree\nlabel: {de: B, en: B}\ntraits: [list]\nlist:\n  tree: true\n  entries:\n    - {key: child, name: C, parent: nope}\n",
	})
	got := strings.Join(problems(t, Validate(p, nil)), "\n")
	for _, want := range []string{
		`type plain: "list" settings need traits: [list]`,
		`type odd: unknown trait "shiny"`,
		`type odd: trait "list" listed twice`,
		`key "Bad-Key" must be snake_case`,
		`type odd, entry b: needs a name`,
		`type odd, entry b: key listed twice`,
		`type odd, entry b: parent needs tree: true`,
		`type odd, entry b: lat, lon and alt need coordinates: true`,
		`type tree, entry child: parent "nope" must be the key of an earlier entry`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
