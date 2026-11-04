// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
)

// --- start data (SD-Plan N9, E6) ---
//
// A package brings start data below data/*.yaml: calendars, deadlines, reasons to wait as
// records of a type, escalation levels as rules. Each entry has a key of the package; a value
// "@key" names an entry before it, "@package.key" one of a package this one depends on, and
// "<package.type>:@key" names it as the recipient of a notification ("core.group:@verteiler",
// SD5). The core creates them once per tenant and then leaves them to the customer
// (internal/seed).

// Seed is one entry of the start data: a record of Type with Values, or a Rule.
type Seed struct {
	Key    string         `json:"key" yaml:"key"`
	Type   string         `json:"type,omitempty" yaml:"type"`
	Values map[string]any `json:"values,omitempty" yaml:"values"`
	Rule   map[string]any `json:"rule,omitempty" yaml:"rule"`
}

// SeedRef splits a value that names another entry: "@key" or "@package.key", also after the
// type of a recipient, "core.group:@key".
func SeedRef(v any) (pkg, key string, ok bool) {
	_, ref, ok := seedRef(v)
	if !ok {
		return "", "", false
	}
	if p, k, dotted := strings.Cut(ref, "."); dotted {
		return p, k, true
	}
	return "", ref, true
}

// runtimeRefs are values with an "@" that the core fills in when it runs, not names of
// entries: the record a rule acts on and who made the write (SD9), the person of the session
// and their groups, the moment of a query.
var runtimeRefs = map[string]bool{"@record": true, "@actor": true, "@me": true, "@my_groups": true, "@now": true}

// seedRef splits a reference into the type before it ("core.group:"; empty for a plain "@")
// and the name after the "@".
func seedRef(v any) (prefix, ref string, ok bool) {
	s, isText := v.(string)
	if !isText || runtimeRefs[s] {
		return "", "", false
	}
	if strings.HasPrefix(s, "@") {
		return "", s[1:], true
	}
	head, ref, found := strings.Cut(s, ":@")
	if !found {
		return "", "", false
	}
	pkg, name, dotted := strings.Cut(head, ".")
	if !dotted || !IsIdent(pkg) || !IsIdent(name) {
		return "", "", false
	}
	return head + ":", ref, true
}

// loadData reads data/*.yaml in the order of the file names, each a list of entries.
func loadData(fsys fs.FS, p *Package) error {
	files, err := fs.ReadDir(fsys, "data")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("data/: %w", err)
	}
	for _, e := range files {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		var list []Seed
		if err := readYAML(fsys, path.Join("data", name), &list); err != nil {
			return err
		}
		p.Data = append(p.Data, list...)
	}
	return nil
}

// validateData checks the start data: keys, one of type and rule, known types, and that every
// "@" names an entry before it or one of a package depended on.
func validateData(p Package, local, known map[string]bool, add func(string, string, ...any)) {
	seen := map[string]bool{}
	for _, s := range p.Data {
		where := "data " + s.Key
		if !IsIdent(s.Key) {
			add(where, "key %q must be snake_case (a-z, 0-9, _), at most 63 characters", s.Key)
		}
		if seen[s.Key] {
			add(where, "defined twice")
		}
		switch {
		case (s.Type == "") == (s.Rule == nil):
			add(where, "an entry is either a record (type and values) or a rule")
		case s.Rule != nil:
			if _, named := s.Rule["name"]; named {
				add(where, "a rule takes its name from the key")
			}
			if s.Values != nil {
				add(where, "a rule has no values")
			}
		case !local[s.Type] && !known[s.Type]:
			add(where, "type %q is neither a type of this package nor installed", s.Type)
		}
		check := func(v any) {
			pkg, key, ok := SeedRef(v)
			switch {
			case !ok:
			case pkg == "" && !seen[key]:
				add(where, "%q names no entry before it", v)
			case pkg != "" && pkg != p.Name && !slices.Contains(p.Depends, pkg):
				add(where, "%q names package %s, which this package does not depend on", v, pkg)
			case pkg == p.Name && !seen[key]:
				add(where, "%q names no entry before it", v)
			}
		}
		walkValues(s.Values, check)
		walkValues(s.Rule, check)
		seen[s.Key] = true
	}
}

// walkValues calls fn with every scalar below v.
func walkValues(v any, fn func(any)) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walkValues(x[k], fn)
		}
	case []any:
		for _, e := range x {
			walkValues(e, fn)
		}
	default:
		fn(v)
	}
}

// ResolveSeedRefs returns v with every "@" replaced by what ids gives for it (keys as
// "package.key"); a name it does not know is an error.
func ResolveSeedRefs(v any, pkg string, ids map[string]string) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			r, err := ResolveSeedRefs(e, pkg, ids)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			r, err := ResolveSeedRefs(e, pkg, ids)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	}
	p, key, ok := SeedRef(v)
	if !ok {
		return v, nil
	}
	if p == "" {
		p = pkg
	}
	id, found := ids[p+"."+key]
	if !found {
		return nil, fmt.Errorf("%v names %s.%s, which is not there", v, p, key)
	}
	prefix, _, _ := seedRef(v)
	return prefix + id, nil
}
