// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authz

import (
	"fmt"
	"regexp"
	"strings"

	"cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/operators"
	"cel.dev/cel-go/common/types"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/rules"
)

// Conditions of permissions are CEL over `record` (PL9; architektur.md §5) and must translate
// to SQL, because lists filter in the database. The subset: comparisons of a field with a
// literal (==, !=, <, <=, >, >=), `in` with a list of literals, `&&`, `||`, `!`, parentheses,
// `null`. Anything else is refused when the permission is saved.

var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Translate turns a condition into a SQL fragment; the placeholders continue after len(args).
// An empty condition is TRUE.
func Translate(condition string, args []any) (string, []any, error) {
	if strings.TrimSpace(condition) == "" {
		return "TRUE", args, nil
	}
	tree, err := rules.AST(condition)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	tr := &translator{args: args}
	sql, err := tr.expr(tree.Expr())
	if err != nil {
		return "", nil, fmt.Errorf("%w: condition %q: %w", ErrInvalid, condition, err)
	}
	return sql, tr.args, nil
}

type translator struct {
	args []any
}

func (t *translator) expr(e ast.Expr) (string, error) {
	switch e.Kind() {
	case ast.CallKind:
		return t.call(e.AsCall())
	default:
		return "", fmt.Errorf("only comparisons, &&, || and ! are allowed, not %v", kindName(e))
	}
}

func (t *translator) call(c ast.CallExpr) (string, error) {
	switch c.FunctionName() {
	case operators.LogicalAnd, operators.LogicalOr:
		op := " AND "
		if c.FunctionName() == operators.LogicalOr {
			op = " OR "
		}
		var parts []string
		for _, arg := range c.Args() {
			s, err := t.expr(arg)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return "(" + strings.Join(parts, op) + ")", nil
	case operators.LogicalNot:
		s, err := t.expr(c.Args()[0])
		if err != nil {
			return "", err
		}
		return "(NOT " + s + ")", nil
	case operators.Equals, operators.NotEquals, operators.Less, operators.LessEquals, operators.Greater, operators.GreaterEquals:
		return t.compare(c)
	case operators.In:
		return t.in(c)
	default:
		return "", fmt.Errorf("function %s is not allowed", c.FunctionName())
	}
}

// compare handles record.field <op> literal (and literal <op> record.field).
func (t *translator) compare(c ast.CallExpr) (string, error) {
	args := c.Args()
	if len(args) != 2 {
		return "", fmt.Errorf("%s needs two sides", c.FunctionName())
	}
	left, right := args[0], args[1]
	col, ok := column(left)
	flipped := false
	if !ok {
		col, ok = column(right)
		left, right = right, left
		flipped = true
	}
	if !ok {
		return "", fmt.Errorf("one side of %s must be a field of the record", c.FunctionName())
	}
	_ = left
	val, isNull, err := literal(right)
	if err != nil {
		return "", err
	}
	op := map[string]string{
		operators.Equals: "=", operators.NotEquals: "<>", operators.Less: "<", operators.LessEquals: "<=",
		operators.Greater: ">", operators.GreaterEquals: ">=",
	}[c.FunctionName()]
	if flipped {
		op = map[string]string{"<": ">", "<=": ">=", ">": "<", ">=": "<=", "=": "=", "<>": "<>"}[op]
	}
	if isNull {
		switch op {
		case "=":
			return col + " IS NULL", nil
		case "<>":
			return col + " IS NOT NULL", nil
		default:
			return "", fmt.Errorf("null can only be compared with == or !=")
		}
	}
	t.args = append(t.args, val)
	return fmt.Sprintf("%s %s $%d", col, op, len(t.args)), nil
}

// in handles record.field in [literals].
func (t *translator) in(c ast.CallExpr) (string, error) {
	args := c.Args()
	if len(args) != 2 {
		return "", fmt.Errorf("in needs two sides")
	}
	col, ok := column(args[0])
	if !ok || args[1].Kind() != ast.ListKind {
		return "", fmt.Errorf("in must be record.field in [values]")
	}
	var ph []string
	for _, el := range args[1].AsList().Elements() {
		val, isNull, err := literal(el)
		if err != nil {
			return "", err
		}
		if isNull {
			return "", fmt.Errorf("null is not allowed in a list")
		}
		t.args = append(t.args, val)
		ph = append(ph, fmt.Sprintf("$%d", len(t.args)))
	}
	if len(ph) == 0 {
		return "FALSE", nil
	}
	return fmt.Sprintf("%s IN (%s)", col, strings.Join(ph, ", ")), nil
}

// column accepts record.<field> and returns the quoted column.
func column(e ast.Expr) (string, bool) {
	if e.Kind() != ast.SelectKind {
		return "", false
	}
	sel := e.AsSelect()
	if sel.IsTestOnly() || sel.Operand().Kind() != ast.IdentKind || sel.Operand().AsIdent() != "record" {
		return "", false
	}
	if !identRe.MatchString(sel.FieldName()) {
		return "", false
	}
	return pgx.Identifier{sel.FieldName()}.Sanitize(), true
}

// literal returns the Go value of a CEL literal; isNull for null.
func literal(e ast.Expr) (any, bool, error) {
	if e.Kind() != ast.LiteralKind {
		return nil, false, fmt.Errorf("the other side must be a literal value, not %v", kindName(e))
	}
	v := e.AsLiteral()
	switch v.Type() {
	case types.NullType:
		return nil, true, nil
	case types.StringType:
		return string(v.(types.String)), false, nil
	case types.IntType:
		return int64(v.(types.Int)), false, nil
	case types.UintType:
		return int64(v.(types.Uint)), false, nil //nolint:gosec // CEL uint literals in conditions are small
	case types.DoubleType:
		return float64(v.(types.Double)), false, nil
	case types.BoolType:
		return bool(v.(types.Bool)), false, nil
	}
	return nil, false, fmt.Errorf("literal of type %s is not allowed", v.Type().TypeName())
}

func kindName(e ast.Expr) string {
	switch e.Kind() {
	case ast.IdentKind:
		return "identifier " + e.AsIdent()
	case ast.SelectKind:
		return "field access"
	case ast.LiteralKind:
		return "literal"
	case ast.ListKind:
		return "list"
	case ast.MapKind:
		return "map"
	case ast.ComprehensionKind:
		return "comprehension"
	}
	return "expression"
}
