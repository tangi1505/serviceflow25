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
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// Load reads a package directory: package.yaml, types/*.yaml, traits/, relations/ and data/.
// It parses strictly (unknown keys are errors) but does not validate; call Validate on the
// result.
func Load(dir string) (Package, error) {
	return LoadFS(os.DirFS(dir))
}

// LoadFS is Load on any file system (embedded packages, tests).
func LoadFS(fsys fs.FS) (Package, error) {
	var p Package
	if err := readYAML(fsys, "package.yaml", &p); err != nil {
		return p, err
	}
	entries, err := fs.ReadDir(fsys, "types")
	if err != nil && !os.IsNotExist(err) {
		return p, fmt.Errorf("types/: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		var t Type
		if err := readYAML(fsys, path.Join("types", name), &t); err != nil {
			return p, err
		}
		if want := strings.TrimSuffix(name, ".yaml"); t.Name != want {
			return p, fmt.Errorf("types/%s: name %q must equal the file name %q", name, t.Name, want)
		}
		if t.IsList() && len(t.Fields) > 0 {
			return p, fmt.Errorf("types/%s: a list takes its fields from the trait; remove \"fields\"", name)
		}
		expandList(&t, p.Name)
		for i := range t.Fields {
			// A rollup is kept by the core like the sums of the costs (WF13).
			if t.Fields[i].Rollup != nil {
				t.Fields[i].Computed = true
			}
		}
		t.Package = p.Name
		p.Types = append(p.Types, t)
	}
	sortTypes(p.Types)
	traits, err := fs.ReadDir(fsys, "traits")
	if err != nil && !os.IsNotExist(err) {
		return p, fmt.Errorf("traits/: %w", err)
	}
	for _, e := range traits {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		var tr Trait
		if err := readYAML(fsys, path.Join("traits", name), &tr); err != nil {
			return p, err
		}
		if want := strings.TrimSuffix(name, ".yaml"); tr.Name != want {
			return p, fmt.Errorf("traits/%s: name %q must equal the file name %q", name, tr.Name, want)
		}
		tr.Package = p.Name
		p.Traits = append(p.Traits, tr)
	}
	sort.Slice(p.Traits, func(i, j int) bool { return p.Traits[i].Name < p.Traits[j].Name })
	countedBy(p)
	for i := range p.Types {
		if err := expandTraits(&p.Types[i], p); err != nil {
			return p, fmt.Errorf("types/%s.yaml: %w", p.Types[i].Name, err)
		}
	}
	rels, err := fs.ReadDir(fsys, "relations")
	if err != nil && !os.IsNotExist(err) {
		return p, fmt.Errorf("relations/: %w", err)
	}
	for _, e := range rels {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		var k RelationKind
		if err := readYAML(fsys, path.Join("relations", name), &k); err != nil {
			return p, err
		}
		if want := strings.TrimSuffix(name, ".yaml"); k.Name != want {
			return p, fmt.Errorf("relations/%s: name %q must equal the file name %q", name, k.Name, want)
		}
		k.Package = p.Name
		p.Relations = append(p.Relations, k)
	}
	sort.Slice(p.Relations, func(i, j int) bool { return p.Relations[i].Name < p.Relations[j].Name })
	for i := range p.Settings {
		p.Settings[i].Package = p.Name
	}
	if err := loadData(fsys, &p); err != nil {
		return p, err
	}
	return p, nil
}

func readYAML(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := yaml.UnmarshalWithOptions(b, v, yaml.Strict()); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
