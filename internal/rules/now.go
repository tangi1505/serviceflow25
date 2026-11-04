// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package rules

import (
	"fmt"
	"time"
	_ "time/tzdata" // the zone of today() also where the system has no zone files

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
)

// "Jetzt" in conditions (SD16d, SD-Plan E11): today() is the date of today as 2006-01-02,
// today(n) the one n days later (negative: earlier); now() is the moment in UTC as RFC 3339,
// now("-24h") the one a duration later. They yield text in the shape the record has for dates
// and moments, so that record.expires_on < today() compares the same in memory as in SQL,
// where the translators of the permissions (PL9) pass the value as a parameter.

// Zone is the time zone of today(): the default of the interface (prefs.DefaultTimeZone),
// which this package cannot import.
const Zone = "Europe/Berlin"

// Clock gives the moment of now() and today(); tests set it.
var Clock = time.Now

var zone = func() *time.Location {
	loc, err := time.LoadLocation(Zone)
	if err != nil {
		panic(err)
	}
	return loc
}()

// Today is the date n days from today in Zone.
func Today(n int) string {
	return Clock().In(zone).AddDate(0, 0, n).Format(time.DateOnly)
}

// Now is the moment a duration from now, as the API writes moments (UTC, RFC 3339).
func Now(offset string) (string, error) {
	t := Clock()
	if offset != "" {
		d, err := time.ParseDuration(offset)
		if err != nil {
			return "", fmt.Errorf("now(%q): a duration such as \"-24h\" or \"30m\": %w", offset, err)
		}
		t = t.Add(d)
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

// timeFunctions declares today() and now() for the environments over a record.
var timeFunctions = []cel.EnvOption{
	cel.Function("today",
		cel.Overload("today_void", nil, cel.StringType,
			cel.FunctionBinding(func(...ref.Val) ref.Val { return types.String(Today(0)) })),
		cel.Overload("today_int", []*cel.Type{cel.IntType}, cel.StringType,
			cel.UnaryBinding(func(v ref.Val) ref.Val { return types.String(Today(int(v.(types.Int)))) })),
	),
	cel.Function("now",
		cel.Overload("now_void", nil, cel.StringType,
			cel.FunctionBinding(func(...ref.Val) ref.Val {
				s, _ := Now("")
				return types.String(s)
			})),
		cel.Overload("now_string", []*cel.Type{cel.StringType}, cel.StringType,
			cel.UnaryBinding(func(v ref.Val) ref.Val {
				s, err := Now(string(v.(types.String)))
				if err != nil {
					return types.NewErr("%s", err.Error())
				}
				return types.String(s)
			})),
	),
}
