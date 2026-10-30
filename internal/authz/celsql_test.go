// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authz

import (
	"errors"
	"reflect"
	"testing"
)

func TestTranslate(t *testing.T) {
	cases := []struct {
		cel  string
		sql  string
		args []any
	}{
		{``, `TRUE`, nil},
		{`record.state == "open"`, `"state" = $1`, []any{"open"}},
		{`record.priority <= 3`, `"priority" <= $1`, []any{int64(3)}},
		{`3 < record.priority`, `"priority" > $1`, []any{int64(3)}},
		{`record.active == true && record.kind != "wish"`, `("active" = $1 AND "kind" <> $2)`, []any{true, "wish"}},
		{`record.kind in ["fault", "question"] || !(record.priority > 4.5)`, `("kind" IN ($1, $2) OR (NOT "priority" > $3))`, []any{"fault", "question", 4.5}},
		{`record.parent == null`, `"parent" IS NULL`, nil},
		{`record.parent != null`, `"parent" IS NOT NULL`, nil},
	}
	for _, c := range cases {
		sql, args, err := Translate(c.cel, nil)
		if err != nil {
			t.Errorf("%q: %v", c.cel, err)
			continue
		}
		if sql != c.sql {
			t.Errorf("%q: got %s, want %s", c.cel, sql, c.sql)
		}
		if len(args) != len(c.args) || (len(args) > 0 && !reflect.DeepEqual(args, c.args)) {
			t.Errorf("%q: args %#v, want %#v", c.cel, args, c.args)
		}
	}
	// Placeholders continue after the arguments already there.
	sql, args, err := Translate(`record.state == "open"`, []any{"x", "y"})
	if err != nil || sql != `"state" = $3` || len(args) != 3 {
		t.Fatalf("offset: %s %v %v", sql, args, err)
	}
}

func TestTranslateRefusesWhatSQLCannotDo(t *testing.T) {
	for _, cel := range []string{
		`record.title.startsWith("x")`,
		`size(record.title) > 3`,
		`record.a == record.b`,
		`record.priority`,
		`"open" == "open"`,
		`record.parent < null`,
		`record.kind in [null]`,
		`has(record.title)`,
		`record.nope +`,
	} {
		if _, _, err := Translate(cel, nil); err == nil || !errors.Is(err, ErrInvalid) {
			t.Errorf("%q should be refused, got %v", cel, err)
		}
	}
}
