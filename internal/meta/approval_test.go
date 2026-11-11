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

// The trait approvals gives a type the computed field approval; the block approval needs the
// trait and names transitions of the type (WF6).
func TestApprovalSpec(t *testing.T) {
	good := `name: order
label: {de: Bestellung, en: Order}
traits: [approvals]
fields:
  - {name: title, type: text, label: {de: T, en: T}}
states:
  - {name: draft, label: {de: N, en: N}, initial: true}
  - {name: done, label: {de: D, en: D}}
transitions:
  - {name: grant, label: {de: G, en: G}, from: [draft], to: done}
approval: {approved: grant}
`
	bad := `name: bad
label: {de: Schlecht, en: Bad}
fields:
  - {name: title, type: text, label: {de: T, en: T}}
states:
  - {name: draft, label: {de: N, en: N}, initial: true}
approval:
  approved: grant
  rejected: nothing
  hold: {fields: [missing], approvers: nobody, when: "record.("}
`
	p, err := LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte(manifest)},
		"types/order.yaml": {Data: []byte(good)},
		"types/bad.yaml":   {Data: []byte(bad)},
	})
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(problems(t, Validate(p, nil)), "\n")
	for _, want := range []string{
		`type bad, approval: needs the trait approvals`,
		`type bad, approval: approved "grant" is not a transition of this type`,
		`type bad, approval: rejected "nothing" is not a transition of this type`,
		// SD17: what a hold names (SD-Plan N22).
		`type bad, approval, hold: field "missing" is not a field of the type that one writes`,
		`type bad, approval, hold: approvers "nobody" is no role of the type for persons or groups`,
		`type bad, approval, hold: when: `,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if strings.Contains(all, "type order") {
		t.Errorf("order is fine:\n%s", all)
	}
	for _, ty := range p.Types {
		if ty.Name != "order" {
			continue
		}
		f, ok := ty.Field(FieldApproval)
		if !ok || !f.Computed || f.Type != FieldChoice || f.Trait != TraitApprovals || len(f.Options) != 4 || !ty.HasApprovals() {
			t.Errorf("the field approval of order: %+v", f)
		}
		if subs := ty.SubResources(); len(subs) != 1 || subs[0] != "approvals" {
			t.Errorf("sub-resources: %v", subs)
		}
	}
}
