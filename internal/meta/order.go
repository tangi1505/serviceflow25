// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"sort"
	"strings"
)

// OrderSpec makes the records of a type orderable (SD13, SD-Plan N19): an offer of the catalog
// names the form a person fills in to order it, and ordering creates the form's record (a
// request) with the values the offer hands on and, where named, a relation from the new record
// to the offer. Form names the reference field to core.form; Copy maps fields of the new record
// to fields of this one (the template of the offer becomes the template of the request);
// Relation is a relation kind, "package.name", from the new record to this one.
type OrderSpec struct {
	Form     string            `json:"form" yaml:"form"`
	Copy     map[string]string `json:"copy,omitempty" yaml:"copy"`
	Relation string            `json:"relation,omitempty" yaml:"relation"`
}

// typeForm is the core type of the forms an order fills in.
const typeForm = "core.form"

// validateOrder checks the block order: a reference field to the forms, fields of this type to
// copy, a relation kind written with its package. Whether the target of a form has the copied
// fields and the kind links them is checked when ordering, since the form is a record.
func validateOrder(add func(string, string, ...any), where string, t Type) {
	o := t.Order
	if o == nil {
		return
	}
	w := where + ", order"
	if f, ok := t.Field(o.Form); !ok || f.Type != FieldReference || f.To != typeForm {
		add(w, "form %q must be a reference field of the type to %s", o.Form, typeForm)
	}
	targets := make([]string, 0, len(o.Copy))
	for target := range o.Copy {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		if !IsIdent(target) {
			add(w, "copy: %q is not a field name", target)
		}
		if _, ok := t.Field(o.Copy[target]); !ok && o.Copy[target] != "id" {
			add(w, "copy: %q is not a field of the type", o.Copy[target])
		}
	}
	if o.Relation != "" {
		pkg, name, ok := strings.Cut(o.Relation, ".")
		if !ok || !IsIdent(pkg) || !IsIdent(name) {
			add(w, "relation %q must be written as package.kind", o.Relation)
		}
	}
}
