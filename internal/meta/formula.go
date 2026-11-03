// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"fmt"
	"sort"
	"strings"

	"group24.de/itsm/internal/rules"
)

// --- calculated fields (KF6; Hauptplan E45, E99; PL22) ---
//
// A field with a formula is computed by the core whenever a record is written and stored like
// any field: it can be filtered and sorted, but not written. The formula is CEL over the other
// fields of the type, each a variable of its own name (impact * urgency). It may use other
// calculated fields; the core computes them in the order of their dependencies and refuses
// cycles. The sums of the trait costs change without a write and cannot be used.

// Calculated reports whether the core computes the field: a sum of the costs, a formula or a
// running number (PL49). Such a field cannot be written through the API.
func (f Field) Calculated() bool { return f.Computed || f.Formula != "" || f.Type == FieldSequence }

// FormulaKind is the CEL kind of a field's values.
func FormulaKind(t FieldType) (rules.Kind, bool) {
	switch t {
	case FieldText, FieldChoice, FieldReference:
		return rules.KindString, true
	case FieldNumber:
		return rules.KindDouble, true
	case FieldBoolean:
		return rules.KindBool, true
	case FieldDate, FieldDateTime:
		return rules.KindTimestamp, true
	}
	return "", false
}

// CompiledFormula is a calculated field with its compiled formula.
type CompiledFormula struct {
	Field   Field
	Formula *rules.Formula
}

// CompileFormulas compiles the formulas of a type in the order they must be computed: a
// formula comes after every calculated field it uses.
func CompileFormulas(t Type) ([]CompiledFormula, error) {
	vars := map[string]rules.Kind{}
	for _, f := range t.Fields {
		if f.Computed {
			continue // the sums of the costs change without a write
		}
		if k, ok := FormulaKind(f.Type); ok {
			vars[f.Name] = k
		}
	}
	byName := map[string]CompiledFormula{}
	var names []string
	for _, f := range t.Fields {
		if f.Formula == "" {
			continue
		}
		if f.Type == FieldReference {
			return nil, fmt.Errorf("field %s: a reference cannot be calculated", f.Name)
		}
		own := make(map[string]rules.Kind, len(vars))
		for n, k := range vars {
			if n != f.Name {
				own[n] = k
			}
		}
		k, _ := FormulaKind(f.Type)
		c, err := rules.CompileFormula(f.Formula, own, k)
		if err != nil {
			return nil, fmt.Errorf("field %s: formula: %w", f.Name, err)
		}
		byName[f.Name] = CompiledFormula{Field: f, Formula: c}
		names = append(names, f.Name)
	}
	// Order by dependencies (depth first); a field met again on its own path is a cycle.
	var out []CompiledFormula
	state := map[string]int{} // 1 on the path, 2 done
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("formulas form a cycle: %s", strings.Join(append(path, name), " → "))
		case 2:
			return nil
		}
		state[name] = 1
		for _, used := range byName[name].Formula.Uses {
			if _, calculated := byName[used]; calculated {
				if err := visit(used, append(path, name)); err != nil {
					return err
				}
			}
		}
		state[name] = 2
		out = append(out, byName[name])
		return nil
	}
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// validateFormulas checks the calculated fields of a type when the package is installed.
func validateFormulas(add func(string, string, ...any), where string, t Type) {
	has := false
	for _, f := range t.Fields {
		if f.Formula == "" {
			continue
		}
		has = true
		fw := where + ", field " + f.Name
		if f.Required || f.Default != nil {
			add(fw, "a calculated field cannot be required or have a default")
		}
		if f.Translatable {
			add(fw, "a calculated field cannot be translatable")
		}
	}
	if !has {
		return
	}
	if _, err := CompileFormulas(t); err != nil {
		add(where, "%v", err)
	}
}
