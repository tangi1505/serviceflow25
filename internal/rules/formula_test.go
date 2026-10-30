// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package rules

import (
	"strings"
	"testing"
	"time"
)

var formulaFields = map[string]Kind{
	"impact": KindDouble, "urgency": KindDouble, "kind": KindString, "active": KindBool, "opened": KindTimestamp,
}

// Formulas are type-checked when they are compiled (PL22): names, operators and the result.
func TestCompileFormulaChecksTypes(t *testing.T) {
	f, err := CompileFormula("impact * urgency", formulaFields, KindDouble)
	if err != nil || strings.Join(f.Uses, ",") != "impact,urgency" {
		t.Fatalf("product: %+v %v", f, err)
	}
	for expr, want := range map[string]string{
		"impact * urgncy":        "undeclared reference",
		"impact * kind":          "no matching overload",
		"kind":                   "the field needs double",
		"impact > 2.0":           "the field needs double",
		"impact * urgency +":     "Syntax error",
		`opened + duration("x")`: "",
	} {
		_, err := CompileFormula(expr, formulaFields, KindDouble)
		if err == nil || (want != "" && !strings.Contains(err.Error(), want)) {
			t.Errorf("%q: %v, want %q", expr, err, want)
		}
	}
	// A whole number fits a number field; a map lookup is checked when it is computed.
	if _, err := CompileFormula("3", formulaFields, KindDouble); err != nil {
		t.Errorf("int for double: %v", err)
	}
	m, err := CompileFormula(`{"a": 1.0, "b": "x"}[kind]`, formulaFields, KindDouble)
	if err != nil {
		t.Fatalf("dyn result: %v", err)
	}
	if _, err := m.Eval(map[string]any{"kind": "b"}); err == nil || !strings.Contains(err.Error(), "the field needs double") {
		t.Errorf("a string out of a dyn map for a number: %v", err)
	}
}

func TestEvalFormula(t *testing.T) {
	f, _ := CompileFormula("impact * urgency", formulaFields, KindDouble)
	if v, err := f.Eval(map[string]any{"impact": 2.0, "urgency": 3.0}); err != nil || v != 6.0 {
		t.Fatalf("6: %v %v", v, err)
	}
	i, _ := CompileFormula("2", formulaFields, KindDouble)
	if v, err := i.Eval(nil); err != nil || v != 2.0 {
		t.Fatalf("int as double: %v %v", v, err)
	}
	d, err := CompileFormula(`opened + duration("72h")`, formulaFields, KindTimestamp)
	if err != nil {
		t.Fatal(err)
	}
	opened := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	if v, err := d.Eval(map[string]any{"opened": opened}); err != nil || !v.(time.Time).Equal(opened.Add(72*time.Hour)) {
		t.Fatalf("date plus three days: %v %v", v, err)
	}
	s, _ := CompileFormula(`active ? kind : "none"`, formulaFields, KindString)
	if v, err := s.Eval(map[string]any{"active": false, "kind": "x"}); err != nil || v != "none" {
		t.Fatalf("string: %v %v", v, err)
	}
	z, _ := CompileFormula(`{"a": 1.0}[kind]`, formulaFields, KindDouble)
	if _, err := z.Eval(map[string]any{"kind": "b"}); err == nil {
		t.Fatal("a missing key must be an error")
	}
}
