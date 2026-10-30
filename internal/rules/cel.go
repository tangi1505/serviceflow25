// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package rules evaluates CEL expressions (Hauptplan E16) over a record: the conditions of
// transitions (WF1), later of rules (WF4) and field states (WF2). An expression sees the
// record as the map `record` with its fields and system fields, and must yield true or false.
package rules

import (
	"errors"
	"fmt"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/ast"
)

var (
	env       = mustEnv("record")
	claimsEnv = mustEnv("claims")
	// ruleEnv is the one of the rules (WF4): the record and the names of the fields a write
	// changes (changed; empty for a rule on a schedule).
	ruleEnv = func() *cel.Env {
		e, err := cel.NewEnv(
			cel.Variable("record", cel.MapType(cel.StringType, cel.DynType)),
			cel.Variable("changed", cel.ListType(cel.StringType)),
		)
		if err != nil {
			panic(err)
		}
		return e
	}()
)

func mustEnv(variable string) *cel.Env {
	e, err := cel.NewEnv(
		cel.Variable(variable, cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		panic(err)
	}
	return e
}

// Condition is a compiled boolean expression over one map variable.
type Condition struct {
	Expr     string
	variable string
	program  cel.Program
}

// Compile checks an expression over `record` at definition time: it must parse, type-check
// and be boolean.
func Compile(expr string) (*Condition, error) {
	return compile(env, "record", expr)
}

// CompileClaims compiles an expression over `claims`, what an identity provider said about
// a person (login rules, PL11).
func CompileClaims(expr string) (*Condition, error) {
	return compile(claimsEnv, "claims", expr)
}

// CompileRule compiles the condition of a rule (WF4): over `record` and `changed`, the
// names of the fields the write changes, e.g. `"kind" in changed && record.kind == "fault"`.
func CompileRule(expr string) (*Condition, error) {
	return compile(ruleEnv, "record", expr)
}

func compile(e *cel.Env, variable, expr string) (*Condition, error) {
	ast, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("rules: %q: %w", expr, iss.Err())
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("rules: %q must yield true or false, it yields %s", expr, ast.OutputType())
	}
	prg, err := e.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("rules: %q: %w", expr, err)
	}
	return &Condition{Expr: expr, variable: variable, program: prg}, nil
}

// AST compiles an expression like Compile and returns its tree, for translators such as the
// one that turns permission conditions into SQL (PL9).
func AST(expr string) (*ast.AST, error) {
	checked, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("rules: %q: %w", expr, iss.Err())
	}
	if checked.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("rules: %q must yield true or false, it yields %s", expr, checked.OutputType())
	}
	return checked.NativeRep(), nil
}

// EvalRule applies the condition of a rule to a record and the changed fields.
func (c *Condition) EvalRule(record map[string]any, changed []string) (bool, error) {
	if c == nil {
		return true, nil
	}
	if changed == nil {
		changed = []string{}
	}
	out, _, err := c.program.Eval(map[string]any{"record": record, "changed": changed})
	if err != nil {
		return false, fmt.Errorf("rules: %q: %w", c.Expr, err)
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, errors.New("rules: " + c.Expr + " did not yield true or false")
	}
	return b, nil
}

// Eval applies the condition to a record. A missing field is an error, not false, so that a
// typo in a definition shows up instead of silently blocking a transition.
func (c *Condition) Eval(record map[string]any) (bool, error) {
	if c == nil {
		return true, nil
	}
	out, _, err := c.program.Eval(map[string]any{c.variable: record})
	if err != nil {
		return false, fmt.Errorf("rules: %q: %w", c.Expr, err)
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, errors.New("rules: " + c.Expr + " did not yield true or false")
	}
	return b, nil
}
