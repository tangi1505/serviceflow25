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
	"regexp"
	"slices"
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
  - {name: rank, type: text, label: {de: X, en: X}, format: priority}
  - {name: level, type: number, label: {de: X, en: X}, format: priority}
  - {name: tenant, type: text, label: {de: X, en: X}}
  - {name: ticket, type: sequence, label: {de: X, en: X}, prefix: INC-, required: true}
  - {name: code, type: text, label: {de: X, en: X}, prefix: X-}
  - {name: case_no, type: sequence, label: {de: X, en: X}, prefix: "IN C"}
states:
  - {name: open, label: {de: O, en: O}, initial: true, category: open}
  - {name: odd, label: {de: X, en: X}, category: pending}
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
		// UI29: priority only on numbers, the category of a state from a short list.
		`field rank: format takes "rich" or "icon" on text fields and "priority" on number fields`,
		`state odd: category "pending" must be open, active, waiting or done`,
		// The column of the tenant (PL46) is no field name either (found in UI34).
		`field tenant: name "tenant" is reserved`,
		// PL49: the running number is set by the core, the prefix belongs to it alone.
		`field ticket: a sequence field is set by the core: no required, default or formula`,
		`field code: prefix only applies to sequence fields`,
		`field case_no: prefix must be at most 20 characters without spaces`,
	}
	if strings.Contains(strings.Join(got, "\n"), "field level") {
		t.Errorf("priority on a number field is fine: %v", got)
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

// A transition sets fixed values (SD5): fields of the type, of their kind; not on a transition
// a role takes, which sets no fields.
func TestTransitionSetsFixedValues(t *testing.T) {
	p := pkg(t, map[string]string{"probe": `name: probe
label: {de: Probe, en: Probe}
fields:
  - {name: title, type: text, label: {de: Titel, en: Title}}
  - {name: major, type: boolean, label: {de: M, en: M}}
traits: [actors]
actors:
  - {name: requester, label: {de: A, en: R}, to: [core.person]}
states:
  - {name: open, label: {de: O, en: O}, initial: true}
  - {name: done, label: {de: D, en: D}}
transitions:
  - {name: declare, label: {de: E, en: D}, from: [open], to: open, set: {major: true}}
  - {name: odd, label: {de: X, en: X}, from: [open], to: done, set: {nope: 1, major: "ja"}}
  - {name: mine, label: {de: X, en: X}, from: [open], to: done, by: [requester], set: {major: true}}
`})
	all := strings.Join(problems(t, Validate(p, nil)), "\n")
	for _, w := range []string{
		`set: "nope" is not a field that can be set`,
		`set: major: default must be`,
		`a transition taken by a role sets no fields`,
	} {
		if !strings.Contains(all, w) {
			t.Errorf("missing problem %q in:\n%s", w, all)
		}
	}
	if strings.Contains(all, "transition declare") {
		t.Errorf("declare is fine: %s", all)
	}
}

// A package names its symbol from the fixed list (UI36); another is refused.
func TestPackageIcon(t *testing.T) {
	p := Package{Name: "demo", Version: "1.0.0", Label: Label{"de": "Demo", "en": "Demo"}, Icon: "database",
		Types: []Type{{Name: "thing", Label: Label{"de": "Ding", "en": "Thing"},
			Fields: []Field{{Name: "title", Type: FieldText, Label: Label{"de": "Titel", "en": "Title"}}}}}}
	if err := Validate(p, nil); err != nil {
		t.Fatalf("database: %v", err)
	}
	p.Icon = "https://example.org/logo.svg"
	if err := Validate(p, nil); err == nil || !strings.Contains(err.Error(), "icon") {
		t.Fatalf("a picture from elsewhere: %v", err)
	}
	// The reading area in the portal takes a symbol of the same list (UI41, UI-Plan E43).
	p.Icon = "database"
	p.Types[0].Portal = &Portal{Read: &PortalRead{Label: Label{"de": "Dinge", "en": "Things"}, Fields: []string{"title"}, Icon: "book-open"}}
	if err := Validate(p, nil); err != nil {
		t.Fatalf("portal area with book-open: %v", err)
	}
	p.Types[0].Portal.Read.Icon = "smiley"
	if err := Validate(p, nil); err == nil || !strings.Contains(err.Error(), "portal read: icon") {
		t.Fatalf("portal area with an unknown symbol: %v", err)
	}
	// A text field may be a symbol (UI42, UI-Plan E53), no other kind of field.
	p.Types[0].Portal = nil
	p.Types[0].Fields = append(p.Types[0].Fields, Field{Name: "icon", Type: FieldText, Format: FormatIcon, Label: Label{"de": "Symbol", "en": "Symbol"}})
	if err := Validate(p, nil); err != nil {
		t.Fatalf("text field with format icon: %v", err)
	}
	p.Types[0].Fields[1].Type = FieldNumber
	if err := Validate(p, nil); err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("number field with format icon: %v", err)
	}
}

// The web knows the same symbols as the core (UI42): web/src/app/icons.ts maps each name of
// PackageIcons to a lucide symbol and no other.
func TestIconsOfWeb(t *testing.T) {
	raw, err := os.ReadFile("../../web/src/app/icons.ts")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	start := strings.Index(src, "export const packageIcons")
	end := strings.Index(src[start:], "};")
	if start < 0 || end < 0 {
		t.Fatal("no map packageIcons in icons.ts")
	}
	var web []string
	for _, m := range regexp.MustCompile(`(?m)^\s+"?([a-z-]+)"?:`).FindAllStringSubmatch(src[start:start+end], -1) {
		web = append(web, m[1])
	}
	if !slices.Equal(web, PackageIcons) {
		t.Fatalf("icons.ts has %v, the core %v", web, PackageIcons)
	}
}

// A colour of the organisation (UI39, UI-Plan E51) reaches 4.5 : 1 on white as text; the
// contrast follows WCAG 2.1.
func TestSettingColor(t *testing.T) {
	if c := Contrast("#000000", "#ffffff"); c < 20.99 || c > 21.01 {
		t.Fatalf("black on white: %v", c)
	}
	if c := Contrast("#0B6E80", "#FFFFFF"); c < 5.8 || c > 6.0 {
		t.Fatalf("petrol on white: %v", c)
	}
	st := Setting{Key: "ui.accent", Kind: SettingColor}
	for value, want := range map[string]string{
		`"#767676"`: "",
		`"#777777"`: "4.4 : 1",
		`"#0b6e80"`: "",
		`"teal"`:    "like #0B6E80",
		`"#12345"`:  "like #0B6E80",
		`12`:        "like #0B6E80",
	} {
		err := st.Check([]byte(value))
		if want == "" && err != nil || want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Fatalf("%s: %v", value, err)
		}
	}
}
