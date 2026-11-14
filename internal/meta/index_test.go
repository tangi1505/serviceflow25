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

// A type declares indexes over its fields and some system columns (PL54); what a B-tree cannot
// hold or what does not exist is refused with the place.
func TestIndexesAreChecked(t *testing.T) {
	good := `name: ticket
label: {de: Ticket, en: Ticket}
fields:
  - {name: code, type: text, max_length: 40, label: {de: C, en: C}}
  - {name: kind, type: choice, label: {de: K, en: K}, options: [{value: a, label: {de: A, en: A}}]}
indexes:
  - fields: [state, created_at]
  - fields: [kind, code]
`
	bad := `name: bad
label: {de: Schlecht, en: Bad}
fields:
  - {name: note, type: text, label: {de: N, en: N}}
  - {name: code, type: text, max_length: 40, label: {de: C, en: C}}
indexes:
  - fields: []
  - fields: [nowhere]
  - fields: [note]
  - fields: [org_path]
  - fields: [code, code]
  - fields: [code, state, created_at, updated_at, sla_due_at]
  - fields: [state]
  - fields: [state]
`
	p, err := LoadFS(fstest.MapFS{
		"package.yaml":      {Data: []byte(manifest)},
		"types/ticket.yaml": {Data: []byte(good)},
		"types/bad.yaml":    {Data: []byte(bad)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ty := range p.Types {
		if ty.Name == "ticket" && (len(ty.Indexes) != 2 || strings.Join(ty.Indexes[0].Fields, ",") != "state,created_at") {
			t.Fatalf("indexes of ticket not loaded: %+v", ty.Indexes)
		}
	}
	all := strings.Join(problems(t, Validate(p, nil)), "\n")
	for _, want := range []string{
		`type bad, index 1: needs 1 to 4 fields, has 0`,
		`type bad, index 2: "nowhere" is not a field of the type`,
		`type bad, index 3: text field "note" needs a max_length of at most 500 to be indexed`,
		`type bad, index 4: system column "org_path" cannot be indexed by a package`,
		`type bad, index 5: names "code" twice`,
		`type bad, index 6: needs 1 to 4 fields, has 5`,
		`type bad, index 8: is declared twice`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if strings.Contains(all, "type ticket") {
		t.Errorf("ticket is fine:\n%s", all)
	}
}
