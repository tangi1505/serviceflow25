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

// The fields a type offers in the portal (UI10) must be fields a user can fill, each once.
func TestValidatePortal(t *testing.T) {
	p := pkg(t, map[string]string{"probe": `name: probe
label: {de: Probe, en: Probe}
fields:
  - {name: title, type: text, label: {de: Titel, en: Title}}
  - {name: sum, type: number, label: {de: Summe, en: Sum}, formula: "1"}
portal:
  fields: [title, nothing, sum, title]
`})
	all := strings.Join(problems(t, Validate(p, nil)), "\n")
	for _, want := range []string{
		`portal: field "nothing" is not a field of the type`,
		`portal: field "sum" is kept by the server and cannot be filled`,
		`portal: field "title" is named twice`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	empty := pkg(t, map[string]string{"probe": `name: probe
label: {de: Probe, en: Probe}
fields:
  - {name: title, type: text, label: {de: Titel, en: Title}}
portal:
  fields: []
`})
	if got := strings.Join(problems(t, Validate(empty, nil)), "\n"); !strings.Contains(got, "portal: fields needs at least one field") {
		t.Errorf("empty portal: %s", got)
	}
}
