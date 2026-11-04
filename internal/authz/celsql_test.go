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
	"time"

	"group24.de/itsm/internal/rules"
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

// SD16d: today() and now() are taken when the condition is translated, as parameters.
func TestTranslateTodayAndNow(t *testing.T) {
	rules.Clock = func() time.Time { return time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { rules.Clock = time.Now })
	for _, c := range []struct {
		cel  string
		sql  string
		args []any
	}{
		{`record.expires_on < today()`, `"expires_on" < $1`, []any{"2026-11-04"}},
		{`today(14) == record.expires_on`, `"expires_on" = $1`, []any{"2026-11-18"}},
		{`record.expires_on == null || record.expires_on >= today(-1)`, `("expires_on" IS NULL OR "expires_on" >= $1)`, []any{"2026-11-03"}},
		{`record.updated_at < now("-24h")`, `"updated_at" < $1`, []any{"2026-11-03T10:00:00Z"}},
		{`record.updated_at <= now()`, `"updated_at" <= $1`, []any{"2026-11-04T10:00:00Z"}},
	} {
		sql, args, err := Translate(c.cel, nil)
		if err != nil || sql != c.sql || !reflect.DeepEqual(args, c.args) {
			t.Errorf("%q: %s %#v %v, want %s %#v", c.cel, sql, args, err, c.sql, c.args)
		}
	}
	for _, cel := range []string{`record.updated_at < now("gestern")`, `record.a < today(record.n)`, `record.a < today() + "x"`} {
		if _, _, err := Translate(cel, nil); err == nil {
			t.Errorf("%q should be refused", cel)
		}
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
