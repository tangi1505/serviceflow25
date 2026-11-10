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

// A type says where it is suggested (SD18, SD-Plan N13); readers may comment (N15); a
// relation kind may ask for the right to update only at its source (N16).
func TestSuggestReadersAndRelationUpdate(t *testing.T) {
	note := `name: note
label: {de: Notiz, en: Note}
traits: [tagged, timeline]
fields:
  - {name: title, type: text, label: {de: T, en: T}}
  - {name: body, type: text, label: {de: B, en: B}}
readers:
  - {actions: [read, comment]}
suggest:
  label: {de: Notizen, en: Notes}
  for: ["trait:metatest.tagged", metatest.case]
  from: [title]
  relation: metatest.uses
  apply:
    label: {de: Übernehmen, en: Use}
    set: {answer: "@body", kind: fixed}
`
	bad := `name: bad
label: {de: Schlecht, en: Bad}
fields:
  - {name: title, type: text, label: {de: T, en: T}}
readers:
  - {actions: [comment, delete]}
suggest:
  label: {de: S, en: S}
  for: ["trait:tagged", other.thing, "trait:sd.ticket"]
  from: []
  relation: metatest.nothing
  apply:
    label: {de: Ü, en: U}
    set: {answer: "@nothing"}
`
	kase := "name: case\nlabel: {de: Fall, en: Case}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n"
	p, err := LoadFS(fstest.MapFS{
		"package.yaml":          {Data: []byte(manifest)},
		"traits/tagged.yaml":    {Data: []byte(taggedTrait)},
		"types/note.yaml":       {Data: []byte(note)},
		"types/bad.yaml":        {Data: []byte(bad)},
		"types/case.yaml":       {Data: []byte(kase)},
		"relations/uses.yaml":   {Data: []byte("name: uses\nlabel: {de: U, en: U}\ninverse: {de: V, en: V}\nfrom: [\"*\"]\nto: [metatest.note]\nupdate: from\n")},
		"relations/broken.yaml": {Data: []byte("name: broken\nlabel: {de: B, en: B}\ninverse: {de: B, en: B}\nfrom: [\"*\"]\nto: [\"*\"]\nupdate: to\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(problems(t, Validate(p, nil)), "\n")
	for _, want := range []string{
		`type bad, readers[0]: action "delete" must be one of read, comment`,
		`type bad, readers[0]: action comment needs the trait timeline`,
		`type bad, suggest: for names "trait:tagged"; a trait goes with its package`,
		`type bad, suggest: for names "other.thing", which is neither a type of this package nor installed`,
		`type bad, suggest: for names "trait:sd.ticket" of package sd, which this package does not depend on`,
		`type bad, suggest: from needs at least one field to search with`,
		`type bad, suggest: relation "metatest.nothing" is no relation kind of this package`,
		`type bad, suggest, apply: answer takes "@nothing", which is no field of bad`,
		`relation broken: update must be "from" or left out (both ends), not "to"`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if strings.Contains(all, "type note") || strings.Contains(all, "relation uses") {
		t.Errorf("note and uses are fine:\n%s", all)
	}
	var n, tagged, plain Type
	for _, ty := range p.Types {
		switch ty.Name {
		case "note":
			n = ty
		case "case":
			plain = ty
		}
	}
	tagged = Type{Name: "x", Package: "metatest", Traits: []string{"tagged"}}
	other := Type{Name: "y", Package: "metatest"}
	if !n.Suggest.Suggests(tagged) || !n.Suggest.Suggests(plain) || n.Suggest.Suggests(other) {
		t.Errorf("note is suggested for the tagged type and case only")
	}
	if r := n.Readers[0]; !r.Lets("read") || !r.Lets("comment") || r.Lets("read_private") || (Reader{}).Lets("comment") {
		t.Errorf("what the readers let do: %+v", n.Readers)
	}
}
