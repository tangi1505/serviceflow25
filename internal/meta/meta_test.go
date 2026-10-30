// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
)

const manifest = `name: metatest
version: 0.1.0
label: {de: Metatest, en: Metatest}
`

func pkg(t *testing.T, types map[string]string) Package {
	t.Helper()
	fsys := fstest.MapFS{"package.yaml": {Data: []byte(manifest)}}
	for name, body := range types {
		fsys["types/"+name+".yaml"] = &fstest.MapFile{Data: []byte(body)}
	}
	p, err := LoadFS(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func problems(t *testing.T, err error) []string {
	t.Helper()
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected a validation error, got %v", err)
	}
	out := make([]string, len(ve.Problems))
	for i, p := range ve.Problems {
		out[i] = p.Error()
	}
	return out
}

func TestLoadProbePackageFromRepository(t *testing.T) {
	p, err := Load("../../packages/probe")
	if err != nil {
		t.Fatal(err)
	}
	// The roles and references of probe.probe name types of the core package (WF3, WF11):
	// they are installed from the core package of the repository, not from a list kept here.
	core, err := Load("../../packages/core")
	if err != nil {
		t.Fatal(err)
	}
	installed := map[string]bool{}
	for _, ct := range core.Types {
		installed[ct.QualifiedName()] = true
	}
	if err := Validate(p, installed); err != nil {
		t.Fatal(err)
	}
	ty, ok := p.Type("probe")
	if !ok || len(ty.Fields) < 5 || ty.Package != "probe" {
		t.Fatalf("probe type incomplete: %+v", ty)
	}
}

func TestLoadRejectsUnknownKeysAndWrongFileName(t *testing.T) {
	fsys := fstest.MapFS{
		"package.yaml":     {Data: []byte(manifest + "colour: red\n")},
		"types/probe.yaml": {Data: []byte("name: probe\nlabel: {de: P, en: P}\nfields: []\n")},
	}
	if _, err := LoadFS(fsys); err == nil || !strings.Contains(err.Error(), "colour") {
		t.Fatalf("unknown key must be rejected, got %v", err)
	}
	fsys["package.yaml"] = &fstest.MapFile{Data: []byte(manifest)}
	fsys["types/other.yaml"] = &fstest.MapFile{Data: []byte("name: probe\nlabel: {de: P, en: P}\nfields: []\n")}
	if _, err := LoadFS(fsys); err == nil || !strings.Contains(err.Error(), "file name") {
		t.Fatalf("name must equal the file name, got %v", err)
	}
}

func TestValidateNamesFieldAndReason(t *testing.T) {
	p := pkg(t, map[string]string{"probe": `name: probe
label: {de: Probe, en: Probe}
fields:
  - {name: title, type: text, label: {de: Titel, en: Title}}
  - {name: kind, type: colour, label: {de: Art, en: Kind}}
  - {name: id, type: text, label: {de: X, en: X}}
  - {name: Bad-Name, type: text, label: {de: X, en: X}}
  - {name: title, type: text, label: {de: X, en: X}}
  - {name: size, type: choice, label: {de: X, en: X}, options: [{value: s, label: {de: S}}, {value: s, label: {de: S, en: S}}], default: xl}
  - {name: owner, type: reference, label: {de: X, en: X}, to: core.person}
  - {name: score, type: number, label: {de: X, en: X}, min: 10, max: 1, max_length: 5, default: "7"}
  - {name: note, type: text, label: {de: X}}
`})
	got := problems(t, Validate(p, nil))
	want := []string{
		`field kind: unknown field type "colour", allowed are text, number, boolean, date, datetime, choice, reference`,
		`field id: name "id" is reserved`,
		`field Bad-Name: name "Bad-Name" must be snake_case`,
		`field title: defined twice`,
		`field size, option s: label has no "en" text`,
		`field size: option "s" listed twice`,
		`field size: default "xl" is not one of the options`,
		`field owner: to "core.person" points to a type that is neither in this package nor installed`,
		`field score: max_length only applies to text fields`,
		`field score: min 10 is greater than max 1`,
		`field score: default must be a number`,
		`field note: label has no "en" text`,
	}
	all := strings.Join(got, "\n")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Errorf("missing problem %q in:\n%s", w, all)
		}
	}
}

func TestValidateReferenceToInstalledType(t *testing.T) {
	p := pkg(t, map[string]string{"probe": `name: probe
label: {de: Probe, en: Probe}
fields:
  - {name: owner, type: reference, label: {de: X, en: X}, to: core.person}
`})
	if err := Validate(p, map[string]bool{"core.person": true}); err != nil {
		t.Fatalf("installed type must satisfy the reference: %v", err)
	}
}

func TestValidatePackageLevel(t *testing.T) {
	fsys := fstest.MapFS{"package.yaml": {Data: []byte("name: Probe!\nversion: 1\nlabel: {de: P}\n")}}
	p, err := LoadFS(fsys)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(problems(t, Validate(p, nil)), "\n")
	for _, w := range []string{`name "Probe!"`, `version "1"`, `label has no "en"`, "has no types"} {
		if !strings.Contains(all, w) {
			t.Errorf("missing %q in:\n%s", w, all)
		}
	}
}

func TestStoreRoundTripAndPrune(t *testing.T) {
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.Migrate(ctx, url, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := queries.New(pool)
	// Start clean: the test database persists between runs.
	if _, err := pool.Exec(ctx, "DELETE FROM package WHERE name = 'metatest'"); err != nil {
		t.Fatal(err)
	}

	two := map[string]string{
		"alpha": "name: alpha\nlabel: {de: A, en: A}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n",
		"beta":  "name: beta\nlabel: {de: B, en: B}\nfields:\n  - {name: owner, type: reference, label: {de: O, en: O}, to: metatest.alpha}\n",
	}
	if err := Store(ctx, pool, pkg(t, two)); err != nil {
		t.Fatal(err)
	}
	known, err := InstalledTypes(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if !known["metatest.alpha"] || !known["metatest.beta"] {
		t.Fatalf("types not installed: %v", known)
	}

	// Second install without beta: beta's base definition is pruned, alpha is bumped.
	delete(two, "beta")
	if err := Store(ctx, pool, pkg(t, two)); err != nil {
		t.Fatal(err)
	}
	rows, err := q.ListPackageDefinitions(ctx, "metatest")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "alpha" || rows[0].Version != 2 {
		t.Fatalf("after prune: %+v", rows)
	}
	if err := Store(ctx, pool, pkg(t, map[string]string{"alpha": "name: alpha\nlabel: {de: A, en: A}\nfields:\n  - {name: x, type: nope, label: {de: X, en: X}}\n"})); err == nil {
		t.Fatal("invalid package must not be stored")
	}
	types, err := Types(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ty := range types {
		if ty.QualifiedName() == "metatest.alpha" && len(ty.Fields) == 1 && ty.Fields[0].Name == "title" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stored type not readable: %+v", types)
	}
}
