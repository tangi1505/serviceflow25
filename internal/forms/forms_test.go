// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package forms

import (
	"encoding/json"
	"strings"
	"testing"

	"group24.de/itsm/internal/meta"
)

var target = meta.Type{Package: "t", Name: "ticket", Traits: []string{meta.TraitTimeline}, Fields: []meta.Field{
	{Name: "title", Type: meta.FieldText, Required: true},
	{Name: "kind", Type: meta.FieldChoice, Required: true, Default: "a", Options: []meta.Option{{Value: "a"}, {Value: "b"}}},
	{Name: "sum", Type: meta.FieldNumber, Computed: true},
}}

func problems(t *testing.T, raw string, typ meta.Type) string {
	t.Helper()
	d, err := Parse(raw)
	if err != nil {
		return err.Error()
	}
	var out []string
	for _, p := range Problems(d, typ) {
		out = append(out, p.Message)
	}
	return strings.Join(out, "; ")
}

// A definition is refused for what could not be submitted; a required field with a default
// need not be asked.
func TestProblems(t *testing.T) {
	ok := `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "f", "type": "date", "label": {"de": "Wann"}}]}]}`
	if got := problems(t, ok, target); got != "" {
		t.Fatalf("valid: %s", got)
	}
	for _, c := range []struct{ raw, want string }{
		{`{"sections": []}`, "at least one section"},
		{`{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title", "extra": 1}]}]}`, "unknown field"},
		{`{"sections": [{"id": "s", "title": {}, "questions": [{"id": "q", "field": "title"}]}]}`, "German title"},
		// Since SD9 a question may also ask for a role: the message names all three ways.
		{`{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title", "type": "text"}]}]}`, "only one of them"},
		{`{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "c", "field": "sum"}]}]}`, "calculated"},
		{`{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "s", "field": "kind"}]}]}`, "id of its own"},
		{`{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "c", "type": "choice", "label": {"de": "C"}, "options": [{"value": "x"}, {"value": "x"}]}]}]}`, "value of its own"},
		{`{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q"}]}]}`, "needs a field"},
	} {
		if got := problems(t, c.raw, target); !strings.Contains(got, c.want) {
			t.Fatalf("%s: want %q, got %q", c.raw, c.want, got)
		}
	}
	plain := target
	plain.Traits = nil
	if got := problems(t, ok, plain); !strings.Contains(got, "has none") {
		t.Fatalf("free questions without a timeline: %q", got)
	}
}

// Free answers become German text for the entry, whatever the language of the person
// submitting (E134; UI11 took that language, changed with UI12).
func TestFreeAnswer(t *testing.T) {
	choice := Question{ID: "c", Type: meta.FieldChoice, Options: []meta.Option{{Value: "x", Label: meta.Label{"de": "Iks", "en": "Ex"}}}}
	for _, c := range []struct {
		q    Question
		v    any
		want string
	}{
		{Question{Type: meta.FieldBoolean}, true, "ja"},
		{Question{Type: meta.FieldBoolean}, false, "nein"},
		{Question{Type: meta.FieldNumber}, 2.5, "2.5"},
		{Question{Type: meta.FieldNumber}, float64(10), "10"},
		{Question{Type: meta.FieldNumber}, json.Number("3"), "3"},
		{choice, "x", "Iks"},
		{Question{Type: meta.FieldDate}, "2026-10-29", "2026-10-29"},
	} {
		got, p := freeAnswer(c.q, c.v)
		if p != nil || got != c.want {
			t.Fatalf("%v %v: %q %v", c.q.Type, c.v, got, p)
		}
	}
	if _, p := freeAnswer(choice, "y"); p == nil || p.Code != "choice" {
		t.Fatalf("unknown option: %v", p)
	}
	if _, p := freeAnswer(Question{Type: meta.FieldNumber}, "drei"); p == nil || p.Code != "number" {
		t.Fatalf("text for a number: %v", p)
	}
}

// Conditions (UI12) name only an earlier question, compare with the values it can have, and
// never hide a field the target needs.
func TestConditionProblems(t *testing.T) {
	base := `{"id": "q", "field": "title"}, {"id": "yn", "type": "boolean", "label": {"de": "Ja?"}}`
	for _, c := range []struct{ cond, want string }{
		{`{"question": "yn", "op": "eq", "value": "true"}`, ""},
		{`{"question": "yn", "op": "answered"}`, ""},
		{`{"question": "later", "op": "answered"}`, "not a question before"},
		{`{"question": "yn", "op": "eq", "value": "ja"}`, "must be one of true, false"},
		{`{"question": "yn", "op": "eq"}`, "needs a value"},
		{`{"question": "yn", "op": "empty", "value": "x"}`, "takes no value"},
		{`{"question": "yn", "op": "gt", "value": "1"}`, "must be eq, ne"},
	} {
		raw := `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [` + base +
			`, {"id": "b", "type": "text", "label": {"de": "B"}, "show_if": [` + c.cond + `]}, {"id": "later", "type": "text", "label": {"de": "L"}}]}]}`
		got := problems(t, raw, target)
		if (c.want == "" && got != "") || !strings.Contains(got, c.want) {
			t.Fatalf("%s: want %q, got %q", c.cond, c.want, got)
		}
	}
	// A section with a condition may not hold the title, which the target needs.
	raw := `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "yn", "type": "boolean", "label": {"de": "Ja?"}}]},
	  {"id": "t", "title": {"de": "B"}, "show_if": [{"question": "yn", "op": "eq", "value": "true"}], "questions": [{"id": "q", "field": "title"}]}]}`
	if got := problems(t, raw, target); !strings.Contains(got, "cannot be hidden") {
		t.Fatalf("hidden required field: %q", got)
	}
	// A bound choice compares with the values of its field.
	raw = `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "k", "field": "kind"},
	  {"id": "b", "type": "text", "label": {"de": "B"}, "show_if": [{"question": "k", "op": "eq", "value": "z"}]}]}]}`
	if got := problems(t, raw, target); !strings.Contains(got, "must be one of a, b") {
		t.Fatalf("bound choice: %q", got)
	}
}

// holds compares the answers as text; a missing or empty answer is neither equal nor answered.
func TestHolds(t *testing.T) {
	answers := map[string]any{"yn": true, "n": json.Number("2"), "t": "", "c": "og1"}
	for _, c := range []struct {
		cs   []Condition
		want bool
	}{
		{[]Condition{{Question: "yn", Op: OpEq, Value: "true"}}, true},
		{[]Condition{{Question: "yn", Op: OpEq, Value: "false"}}, false},
		{[]Condition{{Question: "n", Op: OpEq, Value: "2"}}, true},
		{[]Condition{{Question: "t", Op: OpEmpty}}, true},
		{[]Condition{{Question: "gone", Op: OpEmpty}}, true},
		{[]Condition{{Question: "gone", Op: OpNe, Value: "x"}}, true},
		{[]Condition{{Question: "t", Op: OpAnswered}}, false},
		{[]Condition{{Question: "c", Op: OpNe, Value: "og1"}}, false},
		{[]Condition{{Question: "yn", Op: OpEq, Value: "true"}, {Question: "c", Op: OpEq, Value: "eg"}}, false},
		{nil, true},
	} {
		if got := holds(c.cs, answers); got != c.want {
			t.Fatalf("%+v: %v", c.cs, got)
		}
	}
}
