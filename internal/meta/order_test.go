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

// The block order names a reference field to the forms, fields of the type to copy and a
// relation kind with its package; the reading area's facts and sort are fields (SD13, SD-Plan
// N19, N21).
func TestOrderSpec(t *testing.T) {
	good := `name: offer
label: {de: Angebot, en: Offer}
title_field: name
fields:
  - {name: name, type: text, label: {de: N, en: N}}
  - {name: form, type: reference, to: core.form, label: {de: F, en: F}}
  - {name: days, type: number, label: {de: L, en: L}}
portal:
  read: {label: {de: Katalog, en: Catalogue}, fields: [name, days], facts: [days], sort: name}
order:
  form: form
  relation: metatest.ordered
  copy: {title: name, delivery_days: days}
`
	bad := `name: bad
label: {de: Schlecht, en: Bad}
fields:
  - {name: name, type: text, label: {de: N, en: N}}
portal:
  read: {label: {de: Katalog, en: Catalogue}, fields: [name], facts: [nothing, name, name], sort: -nowhere}
order:
  form: name
  relation: ordered
  copy: {title: missing, "no way": name}
`
	p, err := LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte(manifest)},
		"types/offer.yaml": {Data: []byte(good)},
		"types/bad.yaml":   {Data: []byte(bad)},
	})
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(problems(t, Validate(p, map[string]bool{"core.form": true})), "\n")
	for _, want := range []string{
		`type bad, order: form "name" must be a reference field of the type to core.form`,
		`type bad, order: relation "ordered" must be written as package.kind`,
		`type bad, order: copy: "missing" is not a field of the type`,
		`type bad, order: copy: "no way" is not a field name`,
		`type bad, portal read: fact "nothing" is not a field of the type`,
		`type bad, portal read: fact "name" is named twice`,
		`type bad, portal read: sort "-nowhere" is not a field of the type`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if strings.Contains(all, "type offer") {
		t.Errorf("offer is fine:\n%s", all)
	}
}
