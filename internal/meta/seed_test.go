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

// Start data below data/ (SD-Plan N9): read in the order of the files, every "@" checked
// against the entries before it and the packages depended on, and replaced by ids.
func TestStartDataLoadsChecksAndResolves(t *testing.T) {
	thing := "name: thing\nlabel: {de: D, en: T}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n  - {name: other, type: text, label: {de: O, en: O}}\n"
	p, err := LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte(manifest)},
		"types/thing.yaml": {Data: []byte(thing)},
		"data/a.yaml":      {Data: []byte("- {key: first, type: metatest.thing, values: {title: Eins}}\n")},
		"data/b.yaml": {Data: []byte("- {key: second, type: metatest.thing, values: {title: Zwei, other: \"@first\"}}\n" +
			"- key: level\n  rule: {type_name: metatest.thing, actions: [{notify: {to: [\"@second\"]}}]}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(p, nil); err != nil {
		t.Fatal(err)
	}
	if len(p.Data) != 3 || p.Data[0].Key != "first" || p.Data[2].Rule == nil {
		t.Fatalf("data: %+v", p.Data)
	}
	got, err := ResolveSeedRefs(p.Data[2].Rule, "metatest", map[string]string{"metatest.second": "id-2"})
	if err != nil || !strings.Contains(strings.ReplaceAll(fmtAny(got), " ", ""), "id-2") {
		t.Fatalf("resolved: %v %v", got, err)
	}
	if _, err := ResolveSeedRefs(p.Data[1].Values, "metatest", map[string]string{}); err == nil {
		t.Fatal("an unknown name resolved")
	}

	// Wrong: a key twice, a name ahead of its entry, another package not depended on, an
	// entry that is both or neither, an unknown type, a rule with a name of its own.
	bad, err := LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte(manifest)},
		"types/thing.yaml": {Data: []byte(thing)},
		"data/a.yaml": {Data: []byte(`- {key: early, type: metatest.thing, values: {other: "@late"}}
- {key: late, type: metatest.thing, values: {other: "@sd.servicezeit"}}
- {key: late, type: metatest.thing, values: {title: x}}
- {key: both, type: metatest.thing, rule: {type_name: metatest.thing}}
- {key: neither}
- {key: stranger, type: nowhere.thing, values: {title: x}}
- {key: named, rule: {name: mine, type_name: metatest.thing}}
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = Validate(bad, nil)
	for _, want := range []string{
		`data early: "@late" names no entry before it`,
		"data late: \"@sd.servicezeit\" names package sd, which this package does not depend on",
		"data late: defined twice",
		"data both: an entry is either a record (type and values) or a rule",
		"data neither: an entry is either a record (type and values) or a rule",
		`data stranger: type "nowhere.thing" is neither a type of this package nor installed`,
		"data named: a rule takes its name from the key",
	} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %v", want, err)
		}
	}
}

func fmtAny(v any) string {
	var b strings.Builder
	walkValues(v, func(x any) {
		if s, ok := x.(string); ok {
			b.WriteString(s + ",")
		}
	})
	return b.String()
}
