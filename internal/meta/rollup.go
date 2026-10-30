// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"strings"

	"group24.de/itsm/internal/rules"
)

// Rollup operations.
const (
	RollupSum   = "sum"
	RollupCount = "count"
)

// countedBy notes at each type of a package the rollup fields that count it (Type.CountedBy).
func countedBy(p Package) {
	for _, t := range p.Types {
		for _, f := range t.Fields {
			if f.Rollup == nil {
				continue
			}
			for i := range p.Types {
				if p.Types[i].QualifiedName() == f.Rollup.From {
					p.Types[i].CountedBy = append(p.Types[i].CountedBy, t.QualifiedName()+"."+f.Name)
				}
			}
		}
	}
}

// validateRollups checks the rollup fields of a type (WF13): a number field over a type of the
// same package whose reference field points here; sum names a number field, count none; the
// condition compiles. Whether it translates to SQL shows when records count (authz depends on
// meta, not the other way round).
func validateRollups(add func(string, string, ...any), where string, t Type, p Package) {
	for _, f := range t.Fields {
		r := f.Rollup
		if r == nil {
			continue
		}
		fw := where + ", field " + f.Name
		if f.Type != FieldNumber {
			add(fw, "a rollup is a number field")
		}
		if f.Formula != "" || f.Default != nil {
			add(fw, "a rollup has neither a formula nor a default")
		}
		pkg, local, ok := strings.Cut(r.From, ".")
		if !ok || pkg != p.Name {
			add(fw, "rollup from %q must be a type of this package, written as %s.type", r.From, p.Name)
			continue
		}
		from, ok := p.Type(local)
		if !ok {
			add(fw, "rollup from %q: no such type in this package", r.From)
			continue
		}
		via, ok := from.Field(r.Via)
		if !ok || via.Type != FieldReference || via.To != p.Name+"."+t.Name {
			add(fw, "rollup via %q must be a reference field of %s that points to %s.%s", r.Via, r.From, p.Name, t.Name)
		}
		switch r.Op {
		case RollupSum:
			if sf, ok := from.Field(r.Field); !ok || sf.Type != FieldNumber {
				add(fw, "rollup sum needs a number field of %s, not %q", r.From, r.Field)
			}
		case RollupCount:
			if r.Field != "" {
				add(fw, "rollup count takes no field")
			}
		default:
			add(fw, "rollup op must be sum or count, not %q", r.Op)
		}
		if r.Where != "" {
			if _, err := rules.Compile(r.Where); err != nil {
				add(fw, "rollup where: %v", err)
			}
		}
	}
}
