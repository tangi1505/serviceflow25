// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"group24.de/itsm/internal/customer"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// customerPackage writes a package of its own for the test of the customer layer: a type with
// a title, a choice and a note; without withColor the choice is gone (an update).
func customerPackage(t *testing.T, version string, withColor bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "types"), 0o750); err != nil {
		t.Fatal(err)
	}
	color := ""
	if withColor {
		color = `
  - name: color
    type: choice
    label: {de: Farbe, en: Colour}
    options:
      - {value: red, label: {de: Rot, en: Red}}
      - {value: blue, label: {de: Blau, en: Blue}}`
	}
	files := map[string]string{
		"package.yaml": "name: customertest\nversion: " + version + "\nlabel: {de: Kundentest, en: Customer test}\n",
		"types/thing.yaml": `name: thing
label: {de: Ding, en: Thing}
title_field: title
fields:
  - {name: title, type: text, label: {de: Titel, en: Title}, required: true, max_length: 100}
  - {name: note, type: text, label: {de: Notiz, en: Note}}` + color + "\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The acceptance of PL43 over HTTP: a field added in the customer layer is in the type and in
// the table at once; fields of the package change only as far as allowed; a change another one
// needs stays; the layer survives a re-install; an update that removes a changed field is a
// conflict, and a forced update stills the change; the export holds exactly the changes.
func TestCustomerLayerOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	for _, sql := range []string{
		"DROP TABLE IF EXISTS rec_customertest_thing",
		"DELETE FROM package WHERE name = 'customertest'",
		"DELETE FROM schema_change WHERE type_name LIKE 'customertest.%'",
	} {
		if _, err := admin.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	installPackage(t, ctx, admin.rt, customerPackage(t, "0.1.0", true))
	const base = "/api/v1/types/customertest.thing/customer"

	// Without the right customize on core.type: 403.
	dropRole(t, admin, "pl43_reader")
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "pl43_reader", "label": {"de": "pl43"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	reader := timelineLogin(t, admin, "apitest_pl43_reader", "pl43_reader", `{"last_name": "PL43"}`)
	if r := call(t, reader.api, http.MethodGet, base, "", nil); r.code != 403 {
		t.Fatalf("without customize: %d %s", r.code, r.raw)
	}

	version := 0.0
	add := func(change string, want int) resp {
		t.Helper()
		r := call(t, admin, http.MethodPost, base, fmt.Sprintf(`{"version": %d, "change": %s}`, int64(version), change), nil)
		if r.code != want {
			t.Fatalf("%s: %d %s", change, r.code, r.raw)
		}
		if r.code == 200 {
			version = r.body["version"].(float64)
		}
		return r
	}
	typeOf := func() map[string]any {
		t.Helper()
		r := call(t, admin, http.MethodGet, "/api/v1/types/customertest.thing", "", nil)
		if r.code != 200 {
			t.Fatalf("type: %d %s", r.code, r.raw)
		}
		return r.body
	}
	field := func(typ map[string]any, name string) map[string]any {
		for _, f := range typ["fields"].([]any) {
			if m := f.(map[string]any); m["name"] == name {
				return m
			}
		}
		return nil
	}
	if r := call(t, admin, http.MethodGet, base, "", nil); r.code != 200 || r.body["version"] != 0.0 || len(r.body["changes"].([]any)) != 0 {
		t.Fatalf("empty layer: %d %s", r.code, r.raw)
	}
	rev := typeOf()["revision"]

	// A field of its own: in the type, in the table, readable and writable at once.
	r := add(`{"op": "add_field", "field": "k_dienststelle", "new": {"name": "k_dienststelle", "type": "text", "label": {"de": "Dienststelle"}, "max_length": 80}}`, 200)
	if r.body["changes"].([]any)[0].(map[string]any)["status"] != "active" {
		t.Fatalf("status: %s", r.raw)
	}
	typ := typeOf()
	if field(typ, "k_dienststelle") == nil || typ["revision"] == rev {
		t.Fatalf("type after add_field: %v", typ)
	}
	rec := call(t, admin, http.MethodPost, "/api/v1/records/customertest.thing", `{"title": "eins", "k_dienststelle": "Präsidium"}`, nil)
	if rec.code != 201 || rec.body["k_dienststelle"] != "Präsidium" {
		t.Fatalf("record with the new field: %d %s", rec.code, rec.raw)
	}

	// What the customer layer may not do.
	for _, c := range []struct{ change, want string }{
		{`{"op": "add_field", "field": "dienststelle", "new": {"name": "dienststelle", "type": "text", "label": {"de": "D"}}}`, "k_"},
		{`{"op": "add_field", "field": "k_titel", "new": {"name": "k_titel", "type": "text", "label": {"de": "T"}, "formula": "title"}}`, "no formula"},
		{`{"op": "add_field", "field": "k_dienststelle", "new": {"name": "k_dienststelle", "type": "text", "label": {"de": "D"}}}`, "already"},
		{`{"op": "remove_field", "field": "color"}`, "belongs to the package"},
		{`{"op": "change_field", "field": "color", "set": {"max_length": 5}}`, "belongs to the package"},
		{`{"op": "change_field", "field": "color", "set": {"default": "green"}}`, "not one of the options"},
		{`{"op": "hide_field", "field": "title"}`, "title"},
		{`{"op": "add_options", "field": "note", "options": [{"value": "x", "label": {"de": "X"}}]}`, "no choice"},
		{`{"op": "move_field", "field": "note", "after": "nope"}`, "no field nope"},
		{`{"op": "rename_field", "field": "note"}`, "unknown operation"},
	} {
		if r := add(c.change, 422); !strings.Contains(r.raw, c.want) {
			t.Fatalf("%s: want %q in %s", c.change, c.want, r.raw)
		}
	}
	// What it may.
	add(`{"op": "change_field", "field": "title", "set": {"label": {"de": "Betreff"}}}`, 200)
	add(`{"op": "add_options", "field": "color", "options": [{"value": "green", "label": {"de": "Grün"}}]}`, 200)
	add(`{"op": "hide_field", "field": "note"}`, 200)
	add(`{"op": "move_field", "field": "k_dienststelle"}`, 200)
	typ = typeOf()
	names := []string{}
	for _, f := range typ["fields"].([]any) {
		names = append(names, f.(map[string]any)["name"].(string))
	}
	if names[0] != "k_dienststelle" || field(typ, "title")["label"].(map[string]any)["de"] != "Betreff" ||
		len(field(typ, "color")["options"].([]any)) != 3 || field(typ, "note")["hidden"] != true {
		t.Fatalf("effective type: %v", typ)
	}
	// Someone saved in between: 409.
	if r := call(t, admin, http.MethodPost, base, `{"version": 1, "change": {"op": "hide_field", "field": "color"}}`, nil); r.code != 409 {
		t.Fatalf("stale version: %d %s", r.code, r.raw)
	}
	// The added field cannot go while the move needs it; the move can.
	layer := call(t, admin, http.MethodGet, base, "", nil)
	changes := layer.body["changes"].([]any)
	first := changes[0].(map[string]any)["id"].(string)
	last := changes[len(changes)-1].(map[string]any)["id"].(string)
	if r := call(t, admin, http.MethodDelete, fmt.Sprintf("%s/%s?version=%d", base, first, int64(version)), "", nil); r.code != 422 || !strings.Contains(r.raw, "needs it") {
		t.Fatalf("remove what another change needs: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodDelete, fmt.Sprintf("%s/%s?version=%d", base, last, int64(version)), "", nil); r.code != 200 {
		t.Fatalf("remove the move: %d %s", r.code, r.raw)
	} else {
		version = r.body["version"].(float64)
	}

	// The export holds exactly these changes.
	exp := call(t, admin, http.MethodGet, "/api/v1/customer", "", nil)
	var views []customer.View
	if err := json.Unmarshal([]byte(exp.raw), &views); err != nil {
		t.Fatal(err)
	}
	var mine *customer.View
	for i := range views {
		if views[i].Type == "customertest.thing" {
			mine = &views[i]
		}
	}
	if mine == nil || len(mine.Changes) != 4 || mine.Changes[0].Op != meta.OpAddField || mine.Changes[3].Op != meta.OpHideField {
		t.Fatalf("export: %s", exp.raw)
	}

	// Installed again, the base keeps its customer layer.
	installPackage(t, ctx, admin.rt, customerPackage(t, "0.1.0", true))
	if field(typeOf(), "k_dienststelle") == nil {
		t.Fatal("the customer layer is lost after a re-install")
	}
	// An update without color conflicts with the added option; forced, the change is stilled.
	update, err := meta.Load(customerPackage(t, "0.2.0", false))
	if err != nil {
		t.Fatal(err)
	}
	q := queries.New(admin.pool)
	conflicts, err := customer.UpdateConflicts(ctx, q, update)
	if err != nil {
		t.Fatal(err)
	}
	if cs := conflicts["customertest.thing"]; len(cs) != 1 || cs[0].Change.Op != meta.OpAddOptions || !strings.Contains(cs[0].Reason, "no field color") {
		t.Fatalf("conflicts: %+v", conflicts)
	}
	installPackage(t, ctx, admin.rt, customerPackage(t, "0.2.0", false))
	if err := customer.Still(ctx, q, conflicts); err != nil {
		t.Fatal(err)
	}
	layer = call(t, admin, http.MethodGet, base, "", nil)
	statuses := []string{}
	for _, c := range layer.body["changes"].([]any) {
		statuses = append(statuses, c.(map[string]any)["status"].(string))
	}
	if strings.Join(statuses, ",") != "active,active,inactive,active" {
		t.Fatalf("after the forced update: %s", layer.raw)
	}
	if typ := typeOf(); field(typ, "k_dienststelle") == nil || field(typ, "color") != nil {
		t.Fatalf("effective type after the update: %v", typ)
	}
	// A package may not use the prefix of the customer layer.
	dir := customerPackage(t, "0.3.0", false)
	if err := os.WriteFile(filepath.Join(dir, "types", "thing.yaml"), []byte("name: thing\nlabel: {de: D, en: T}\nfields:\n  - {name: k_x, type: text, label: {de: X, en: X}}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	p, err := meta.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, admin.pool, p); err == nil || !strings.Contains(err.Error(), "customer layer") {
		t.Fatalf("package with k_: %v", err)
	}
}
