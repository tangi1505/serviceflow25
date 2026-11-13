// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- settings of the organisation tree (SD20; Hauptplan E131, E180) ---
//
// A package declares the settings it reads in package.yaml: key, kind, label, default and the
// bounds of its values. The page "Globale Einstellungen" shows them by group, the server takes
// only declared keys and checks every value against its declaration. The core declares its
// own (ui., security., files., approvals. …) in the package core.

// SettingKind is the kind of a setting's value.
type SettingKind string

// The kinds of settings. Stored values are JSON: choice and text strings, number a number,
// boolean a bool, duration whole minutes, size whole bytes, list an array of strings, matrix an
// object of rows of objects of columns, reference the id of a record.
const (
	SettingChoice    SettingKind = "choice"
	SettingText      SettingKind = "text"
	SettingTimeZone  SettingKind = "time_zone"
	SettingNumber    SettingKind = "number"
	SettingBoolean   SettingKind = "boolean"
	SettingDuration  SettingKind = "duration"
	SettingSize      SettingKind = "size"
	SettingList      SettingKind = "list"
	SettingMatrix    SettingKind = "matrix"
	SettingReference SettingKind = "reference"
)

// SettingKinds are all kinds, in the order the documentation names them.
var SettingKinds = []SettingKind{SettingChoice, SettingText, SettingTimeZone, SettingNumber, SettingBoolean,
	SettingDuration, SettingSize, SettingList, SettingMatrix, SettingReference}

// SettingOption is one allowed value of a choice or of the cells of a matrix.
type SettingOption struct {
	Value any   `json:"value" yaml:"value"`
	Label Label `json:"label" yaml:"label"`
}

// SettingAxis is the rows or the columns of a matrix: what they mean and their values.
type SettingAxis struct {
	Label   Label           `json:"label" yaml:"label"`
	Options []SettingOption `json:"options" yaml:"options"`
}

// Setting is one declared setting.
type Setting struct {
	// Package is the declaring package; set by the loader.
	Package     string      `json:"package,omitempty" yaml:"-"`
	Key         string      `json:"key" yaml:"key"`
	Kind        SettingKind `json:"kind" yaml:"kind"`
	Label       Label       `json:"label" yaml:"label"`
	Description Label       `json:"description,omitempty" yaml:"description"`
	// Group names the section of the page; empty: the package's label.
	Group Label `json:"group,omitempty" yaml:"group"`
	// Unit is what a number counts (Arbeitstage, Sekunden), for the page only.
	Unit Label `json:"unit,omitempty" yaml:"unit"`
	// Default is the value without a setting at the node or above; nil: none.
	Default any `json:"default,omitempty" yaml:"default"`
	// Min and Max bound a number, a duration (minutes) or a size (bytes).
	Min *float64 `json:"min,omitempty" yaml:"min"`
	Max *float64 `json:"max,omitempty" yaml:"max"`
	// Integer says a number must be whole.
	Integer bool `json:"integer,omitempty" yaml:"integer"`
	// Pattern checks a text or each entry of a list (a regular expression for the whole value).
	Pattern string `json:"pattern,omitempty" yaml:"pattern"`
	// Options are the values of a choice or of the cells of a matrix.
	Options []SettingOption `json:"options,omitempty" yaml:"options"`
	// Rows and Columns are the axes of a matrix.
	Rows    *SettingAxis `json:"rows,omitempty" yaml:"rows"`
	Columns *SettingAxis `json:"columns,omitempty" yaml:"columns"`
	// To is the type a reference names.
	To string `json:"to,omitempty" yaml:"to"`
	// Root says the setting is read at the root node only and cannot be set below it.
	Root bool `json:"root,omitempty" yaml:"root"`
}

var settingKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// SettingKeyValid says whether k is a key of dotted snake_case, like incident.close_after.
func SettingKeyValid(k string) bool { return len(k) <= 120 && settingKeyRe.MatchString(k) }

// checkSettings checks the declarations of a package: keys of the package's own prefix (the
// core is free), known kinds, labels, the parts each kind needs and a default that passes.
func checkSettings(add func(string, string, ...any), p Package) {
	seen := map[string]bool{}
	for _, st := range p.Settings {
		where := "setting " + st.Key
		switch {
		case !SettingKeyValid(st.Key):
			add(where, "key must be dotted snake_case, like %s.close_after", p.Name)
		case p.Name != "core" && !strings.HasPrefix(st.Key, p.Name+"."):
			add(where, "key must start with the package name %q", p.Name+".")
		}
		if seen[st.Key] {
			add(where, "declared twice")
		}
		seen[st.Key] = true
		if !slices.Contains(SettingKinds, st.Kind) {
			add(where, "kind %q is unknown; kinds are choice, text, time_zone, number, boolean, duration, size, list, matrix and reference", st.Kind)
			continue
		}
		checkLabel(add, where, "label", st.Label)
		if st.Pattern != "" {
			if _, err := regexp.Compile(st.Pattern); err != nil {
				add(where, "pattern: %v", err)
			}
		}
		switch st.Kind {
		case SettingChoice:
			if len(st.Options) == 0 {
				add(where, "a choice needs options")
			}
			for _, o := range st.Options {
				if _, ok := o.Value.(string); !ok {
					add(where, "the options of a choice are texts, not %v", o.Value)
				}
			}
		case SettingMatrix:
			if st.Rows == nil || st.Columns == nil || len(st.Rows.Options) == 0 || len(st.Columns.Options) == 0 {
				add(where, "a matrix needs rows and columns with options")
			}
			if len(st.Options) == 0 {
				add(where, "a matrix needs the options of its cells")
			}
		case SettingReference:
			if st.To == "" {
				add(where, "a reference needs to: the type it names")
			}
		}
		if st.Default != nil {
			if err := st.Check(normalJSON(st.Default)); err != nil {
				add(where, "default: %v", err)
			}
		}
	}
}

// normalJSON turns a YAML value into what the same value is as JSON (maps of strings).
func normalJSON(v any) json.RawMessage {
	raw, err := json.Marshal(yamlToJSON(v))
	if err != nil {
		return nil
	}
	return raw
}

func yamlToJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = yamlToJSON(e)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[fmt.Sprint(k)] = yamlToJSON(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = yamlToJSON(e)
		}
		return out
	}
	return v
}

// DefaultJSON is the default as JSON, nil without one.
func (st Setting) DefaultJSON() json.RawMessage {
	if st.Default == nil {
		return nil
	}
	return normalJSON(st.Default)
}

// Check says why raw is no value of the setting, nil when it is one.
func (st Setting) Check(raw json.RawMessage) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("is not JSON")
	}
	if v == nil {
		return fmt.Errorf("must not be null; remove the setting instead")
	}
	switch st.Kind {
	case SettingChoice:
		s, ok := v.(string)
		if !ok || !st.hasOption(s) {
			return fmt.Errorf("must be one of %s", st.optionList())
		}
	case SettingText:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("must be a text")
		}
		if err := st.matches(s); err != nil {
			return err
		}
	case SettingTimeZone:
		s, ok := v.(string)
		if !ok || s == "" || s == "Local" {
			return fmt.Errorf("must be an IANA time zone such as Europe/Berlin")
		}
		if _, err := time.LoadLocation(s); err != nil {
			return fmt.Errorf("must be an IANA time zone such as Europe/Berlin")
		}
	case SettingBoolean:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("must be true or false")
		}
	case SettingNumber, SettingDuration, SettingSize:
		n, ok := v.(float64)
		if !ok {
			return fmt.Errorf("must be a number")
		}
		whole := st.Integer || st.Kind != SettingNumber
		if whole && n != math.Trunc(n) {
			return fmt.Errorf("must be a whole number")
		}
		if st.Kind != SettingNumber && n < 0 {
			return fmt.Errorf("must not be negative")
		}
		if st.Min != nil && n < *st.Min {
			return fmt.Errorf("must be at least %v", *st.Min)
		}
		if st.Max != nil && n > *st.Max {
			return fmt.Errorf("must be at most %v", *st.Max)
		}
	case SettingList:
		list, ok := v.([]any)
		if !ok {
			return fmt.Errorf("must be a list of texts")
		}
		for _, e := range list {
			s, ok := e.(string)
			if !ok || s == "" {
				return fmt.Errorf("must be a list of texts")
			}
			if err := st.matches(s); err != nil {
				return fmt.Errorf("entry %q %w", s, err)
			}
		}
	case SettingMatrix:
		return st.checkMatrix(v)
	case SettingReference:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("must be the id of a record of %s", st.To)
		}
		if _, err := uuid.Parse(s); err != nil {
			return fmt.Errorf("must be the id of a record of %s", st.To)
		}
	}
	return nil
}

func (st Setting) matches(s string) error {
	if st.Pattern == "" {
		return nil
	}
	re, err := regexp.Compile("^(?:" + st.Pattern + ")$")
	if err != nil || !re.MatchString(s) {
		return fmt.Errorf("must match %s", st.Pattern)
	}
	return nil
}

func (st Setting) hasOption(v any) bool {
	for _, o := range st.Options {
		if sameValue(o.Value, v) {
			return true
		}
	}
	return false
}

// sameValue compares an option from YAML (int, string) with a value from JSON (float64, string).
func sameValue(a, b any) bool {
	return fmt.Sprint(yamlToJSON(a)) == fmt.Sprint(b) || fmt.Sprint(toFloat(a)) == fmt.Sprint(toFloat(b))
}

func toFloat(v any) any {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	}
	return v
}

func (st Setting) optionList() string {
	vals := make([]string, len(st.Options))
	for i, o := range st.Options {
		vals[i] = fmt.Sprint(o.Value)
	}
	return strings.Join(vals, ", ")
}

// checkMatrix wants a value for every pair of row and column, each one of the options.
func (st Setting) checkMatrix(v any) error {
	if st.Rows == nil || st.Columns == nil {
		return fmt.Errorf("the setting has no rows or columns")
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("must be an object of rows")
	}
	for _, r := range st.Rows.Options {
		rk := fmt.Sprint(r.Value)
		row, ok := m[rk].(map[string]any)
		if !ok {
			return fmt.Errorf("row %s is missing", rk)
		}
		for _, c := range st.Columns.Options {
			ck := fmt.Sprint(c.Value)
			cell, ok := row[ck]
			if !ok {
				return fmt.Errorf("cell %s/%s is missing", rk, ck)
			}
			if !st.hasOption(cell) {
				return fmt.Errorf("cell %s/%s must be one of %s", rk, ck, st.optionList())
			}
		}
		if len(row) != len(st.Columns.Options) {
			return fmt.Errorf("row %s has columns that are not %s", rk, axisList(st.Columns))
		}
	}
	if len(m) != len(st.Rows.Options) {
		return fmt.Errorf("has rows that are not %s", axisList(st.Rows))
	}
	return nil
}

func axisList(a *SettingAxis) string {
	vals := make([]string, len(a.Options))
	for i, o := range a.Options {
		vals[i] = fmt.Sprint(o.Value)
	}
	return strings.Join(vals, ", ")
}
