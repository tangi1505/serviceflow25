// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package rules

import "testing"

func TestCompileAndEval(t *testing.T) {
	c, err := Compile(`record.priority <= 3 && record.kind != "wish"`)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := c.Eval(map[string]any{"priority": 2.0, "kind": "fault"})
	if err != nil || !ok {
		t.Fatalf("expected true, got %v %v", ok, err)
	}
	ok, err = c.Eval(map[string]any{"priority": 5.0, "kind": "fault"})
	if err != nil || ok {
		t.Fatalf("expected false, got %v %v", ok, err)
	}
	if _, err := c.Eval(map[string]any{"kind": "fault"}); err == nil {
		t.Fatal("a missing field must be an error, not false")
	}
	if ok, err := (*Condition)(nil).Eval(nil); err != nil || !ok {
		t.Fatal("a nil condition is always true")
	}
}

func TestCompileRefusesBadExpressions(t *testing.T) {
	for _, expr := range []string{`record.priority +`, `record.priority`, `"text"`, `record.title.nope(`} {
		if _, err := Compile(expr); err == nil {
			t.Errorf("%q should not compile", expr)
		}
	}
}

// The conditions of rules (WF4) see the changed fields besides the record; other conditions
// do not know them.
func TestRuleConditionSeesChanged(t *testing.T) {
	c, err := CompileRule(`"kind" in changed && record.kind == "fault"`)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := c.EvalRule(map[string]any{"kind": "fault"}, []string{"title", "kind"}); err != nil || !ok {
		t.Fatalf("kind changed to fault: %v %v", ok, err)
	}
	if ok, err := c.EvalRule(map[string]any{"kind": "fault"}, nil); err != nil || ok {
		t.Fatalf("nothing changed: %v %v", ok, err)
	}
	if _, err := Compile(`"kind" in changed`); err == nil {
		t.Fatal("a transition condition must not know changed")
	}
}
