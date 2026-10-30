// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package rules

import (
	"fmt"
	"sort"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
)

// --- formulas of calculated fields (KF6; Hauptplan E45, E99; PL22) ---
//
// A formula sees every field of its type as a variable of its own name with the CEL type of
// the field, so that the definition is type-checked when it is installed: an unknown field,
// an operator that does not fit, or a result that does not fit the field fails there, not
// on the first write.

// Kind is the CEL type of a field value.
type Kind string

// The kinds the fields map to.
const (
	KindString    Kind = "string"    // text, choice, reference
	KindDouble    Kind = "double"    // number
	KindBool      Kind = "bool"      // boolean
	KindTimestamp Kind = "timestamp" // date, datetime
)

func (k Kind) celType() (*cel.Type, error) {
	switch k {
	case KindString:
		return cel.StringType, nil
	case KindDouble:
		return cel.DoubleType, nil
	case KindBool:
		return cel.BoolType, nil
	case KindTimestamp:
		return cel.TimestampType, nil
	}
	return nil, fmt.Errorf("rules: unknown kind %q", k)
}

// Formula is a compiled expression over the fields of one type.
type Formula struct {
	Expr string
	// Uses are the fields the expression reads, in name order.
	Uses    []string
	result  Kind
	program cel.Program
}

// CompileFormula checks expr against the fields (name → kind) and the kind of the result.
// A whole number is accepted for a double; a value of dynamic type (a map lookup) is checked
// when it is computed.
func CompileFormula(expr string, fields map[string]Kind, result Kind) (*Formula, error) {
	opts := make([]cel.EnvOption, 0, len(fields))
	for name, k := range fields {
		t, err := k.celType()
		if err != nil {
			return nil, err
		}
		opts = append(opts, cel.Variable(name, t))
	}
	e, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	checked, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("rules: %q: %w", expr, iss.Err())
	}
	want, err := result.celType()
	if err != nil {
		return nil, err
	}
	out := checked.OutputType()
	fits := out.IsExactType(want) || out.IsExactType(cel.DynType) ||
		(result == KindDouble && out.IsExactType(cel.IntType))
	if !fits {
		return nil, fmt.Errorf("rules: %q yields %s, the field needs %s", expr, out, want)
	}
	seen := map[string]bool{}
	for _, r := range checked.NativeRep().ReferenceMap() {
		if _, ok := fields[r.Name]; ok {
			seen[r.Name] = true
		}
	}
	uses := make([]string, 0, len(seen))
	for name := range seen {
		uses = append(uses, name)
	}
	sort.Strings(uses)
	prg, err := e.Program(checked)
	if err != nil {
		return nil, fmt.Errorf("rules: %q: %w", expr, err)
	}
	return &Formula{Expr: expr, Uses: uses, result: result, program: prg}, nil
}

// Eval computes the formula. vars holds the used fields as Go values: string, float64, bool,
// time.Time. The result is a string, float64, bool or time.Time.
func (f *Formula) Eval(vars map[string]any) (any, error) {
	out, _, err := f.program.Eval(vars)
	if err != nil {
		return nil, fmt.Errorf("rules: %q: %w", f.Expr, err)
	}
	return f.native(out)
}

func (f *Formula) native(v ref.Val) (any, error) {
	switch x := v.(type) {
	case types.String:
		if f.result == KindString {
			return string(x), nil
		}
	case types.Double:
		if f.result == KindDouble {
			return float64(x), nil
		}
	case types.Int:
		if f.result == KindDouble {
			return float64(x), nil
		}
	case types.Bool:
		if f.result == KindBool {
			return bool(x), nil
		}
	case types.Timestamp:
		if f.result == KindTimestamp {
			return x.UTC(), nil
		}
	}
	return nil, fmt.Errorf("rules: %q yielded %v of type %s, the field needs %s", f.Expr, v, v.Type(), f.result)
}
