// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/richtext"
)

// Formats of date and time values in the API: dates as 2006-01-02, timestamps as RFC 3339.
const dateFormat = "2006-01-02"

// coerce turns a JSON value into the Go value stored for the field, or reports a problem.
// nil stays nil (the field is cleared).
func coerce(f meta.Field, v any) (any, *Problem) {
	if v == nil {
		return nil, nil
	}
	bad := func(msg string) (any, *Problem) {
		return nil, &Problem{Field: f.Name, Code: "type", Message: msg}
	}
	switch f.Type {
	case meta.FieldText:
		s, ok := v.(string)
		if !ok {
			return bad("must be a string")
		}
		if f.Format == meta.FormatRich {
			// Formatted text (UI7): only what the policy allows is kept; the limit counts the HTML.
			s = richtext.Sanitize(s)
		}
		if f.MaxLength > 0 && len([]rune(s)) > f.MaxLength {
			return nil, &Problem{Field: f.Name, Code: "max_length", Message: fmt.Sprintf("must have at most %d characters", f.MaxLength)}
		}
		return s, nil
	case meta.FieldChoice:
		s, ok := v.(string)
		if !ok {
			return bad("must be a string")
		}
		for _, o := range f.Options {
			if o.Value == s {
				return s, nil
			}
		}
		return nil, &Problem{Field: f.Name, Code: "choice", Message: fmt.Sprintf("%q is not one of the options", s)}
	case meta.FieldNumber:
		var n float64
		switch x := v.(type) {
		case float64:
			n = x
		case json.Number:
			var err error
			if n, err = x.Float64(); err != nil {
				return bad("must be a number")
			}
		case int:
			n = float64(x)
		case int64:
			n = float64(x)
		default:
			return bad("must be a number")
		}
		if f.Min != nil && n < *f.Min {
			return nil, &Problem{Field: f.Name, Code: "min", Message: fmt.Sprintf("must be at least %v", *f.Min)}
		}
		if f.Max != nil && n > *f.Max {
			return nil, &Problem{Field: f.Name, Code: "max", Message: fmt.Sprintf("must be at most %v", *f.Max)}
		}
		return n, nil
	case meta.FieldBoolean:
		b, ok := v.(bool)
		if !ok {
			return bad("must be true or false")
		}
		return b, nil
	case meta.FieldDate:
		s, ok := v.(string)
		if !ok {
			return bad("must be a date as 2006-01-02")
		}
		d, err := time.Parse(dateFormat, s)
		if err != nil {
			return bad("must be a date as 2006-01-02")
		}
		return d, nil
	case meta.FieldDateTime:
		s, ok := v.(string)
		if !ok {
			return bad("must be a timestamp as RFC 3339")
		}
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return bad("must be a timestamp as RFC 3339")
		}
		return t, nil
	case meta.FieldReference:
		s, ok := v.(string)
		if !ok {
			return bad("must be the id of a " + f.To)
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return bad("must be the id of a " + f.To)
		}
		return id, nil
	case meta.FieldSequence:
		// A running number, with or without its prefix (PL49): for filters and imports.
		n, ok := parseNumber(f, v)
		if !ok {
			return bad("must be a whole number from 1, with or without the prefix " + strconv.Quote(f.Prefix))
		}
		return n, nil
	}
	return bad("unsupported field type")
}

// toJSON turns a scanned column value into its API representation (Hauptplan E70).
func toJSON(f meta.Field, v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case pgtype.Numeric:
		if !x.Valid {
			return nil
		}
		if f8, err := x.Float64Value(); err == nil && f8.Valid {
			return f8.Float64
		}
		return nil
	case *big.Float:
		n, _ := x.Float64()
		return n
	case time.Time:
		if f.Type == meta.FieldDate {
			return x.Format(dateFormat)
		}
		return x.UTC().Format(time.RFC3339Nano)
	case pgtype.Date:
		if !x.Valid {
			return nil
		}
		return x.Time.Format(dateFormat)
	case uuid.UUID:
		return x.String()
	case [16]byte:
		return uuid.UUID(x).String()
	case int64:
		return x
	case float64:
		return x
	case bool:
		return x
	case string:
		return x
	}
	return fmt.Sprint(v)
}

// systemToJSON formats a system column.
func systemToJSON(name string, v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case uuid.UUID:
		return x.String()
	case [16]byte:
		return uuid.UUID(x).String()
	case int64, bool:
		return x
	case string:
		if name == "org_path" {
			return x
		}
		return x
	}
	return fmt.Sprint(v)
}

// equalJSON compares two API representations.
func equalJSON(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch x := a.(type) {
	case float64:
		switch y := b.(type) {
		case float64:
			return x == y
		case int64:
			return x == float64(y)
		case json.Number:
			f, _ := y.Float64()
			return x == f
		}
	case int64:
		return equalJSON(float64(x), b)
	}
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// coerceString parses a query-string value for a filter on the field.
func coerceString(f meta.Field, raw string) (any, *Problem) {
	switch f.Type {
	case meta.FieldNumber:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, &Problem{Field: f.Name, Code: "type", Message: "must be a number"}
		}
		return coerce(f, n)
	case meta.FieldBoolean:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, &Problem{Field: f.Name, Code: "type", Message: "must be true or false"}
		}
		return coerce(f, b)
	default:
		return coerce(f, raw)
	}
}
