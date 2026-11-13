// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"context"
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
	// auth/me tells the interface whether to offer the editor (UI25).
	if r := call(t, reader.api, http.MethodGet, "/api/v1/auth/me", "", nil); r.code != 200 || r.body["customize"] != false || r.body["manage_rights"] != false {
		t.Fatalf("me without customize: %d %s", r.code, r.raw)
	}
	// manage_rights offers the role editor (UI27).
	self := call(t, admin, http.MethodGet, "/api/v1/auth/me", "", nil)
	if self.code != 200 || self.body["customize"] != true || self.body["manage_rights"] != true {
		t.Fatalf("me with customize: %d %s", self.code, self.raw)
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
	if first := r.body["changes"].([]any)[0].(map[string]any); first["status"] != "active" || first["by_name"] != self.body["display_name"] {
		t.Fatalf("status and author: %s", r.raw)
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

// A type with traits and rollups of its package takes a field of the customer layer: the
// change is checked inside its package (found with UI25 on probe.probe, whose trait tagged and
// rollups from probe.task belong to the package probe).
func TestCustomerLayerInsideItsPackage(t *testing.T) {
	admin := apiServer(t)
	drop := func() {
		if _, err := admin.pool.Exec(context.Background(), "DELETE FROM definition WHERE package = 'probe' AND name = 'probe' AND layer = 'customer'"); err != nil {
			t.Fatal(err)
		}
		// Past the API: the types in memory hear of it at once, not only by the notice (PL53).
		meta.DefinitionsChanged()
	}
	drop()
	t.Cleanup(drop)
	const base = "/api/v1/types/probe.probe/customer"
	r := call(t, admin, http.MethodPost, base, `{"version": 0, "change": {"op": "add_field", "field": "k_pl43_paket", "new": {"name": "k_pl43_paket", "type": "text", "label": {"de": "Im Paket"}, "max_length": 40}}}`, nil)
	if r.code != 200 {
		t.Fatalf("add_field on probe.probe: %d %s", r.code, r.raw)
	}
	id := r.body["changes"].([]any)[0].(map[string]any)["id"].(string)
	if r := call(t, admin, http.MethodDelete, base+"/"+id+"?version=1", "", nil); r.code != 200 {
		t.Fatalf("take back: %d %s", r.code, r.raw)
	}

	// The state model (UI26.1): an own state and a transition into it with a condition, a
	// transition of the package disabled and enabled again, a state with records stays.
	version := int64(2)
	add := func(change string, want int) resp {
		t.Helper()
		r := call(t, admin, http.MethodPost, base, fmt.Sprintf(`{"version": %d, "change": %s}`, version, change), nil)
		if r.code != want {
			t.Fatalf("%s: %d %s", change, r.code, r.raw)
		}
		if r.code == 200 {
			version = int64(r.body["version"].(float64))
		}
		return r
	}
	add(`{"op": "add_state", "field": "k_review", "state": {"name": "k_review", "label": {"de": "Prüfung"}}}`, 200)
	add(`{"op": "add_transition", "field": "k_review_it", "transition": {"name": "k_review_it", "label": {"de": "Prüfen"}, "from": ["in_progress"], "to": "k_review", "when": "record.priority <= 2"}}`, 200)
	add(`{"op": "add_transition", "field": "k_back", "transition": {"name": "k_back", "label": {"de": "Zurück"}, "from": ["k_review"], "to": "in_progress"}}`, 200)
	// The layout of the graph (UI26.2): one change with positions, in the effective type at
	// once; a name that is no state is refused.
	add(`{"op": "set_layout", "field": "layout", "layout": {"new": {"x": 0, "y": 0}, "in_progress": {"x": 240, "y": 0}, "k_review": {"x": 480, "y": 140}}}`, 200)
	if r := add(`{"op": "set_layout", "field": "layout", "layout": {"nowhere": {"x": 1, "y": 1}}}`, 422); !strings.Contains(r.raw, "not a state") {
		t.Fatalf("bad layout: %s", r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/types/probe.probe", "", nil); r.code != 200 || !strings.Contains(r.raw, `"k_review":{"x":480,"y":140}`) {
		t.Fatalf("layout in the type: %d %s", r.code, r.raw)
	}
	// A condition that does not compile, and a transition from a state that is not there.
	if r := add(`{"op": "add_transition", "field": "k_bad", "transition": {"name": "k_bad", "label": {"de": "x"}, "from": ["new"], "to": "k_review", "when": "record.priority <"}}`, 422); !strings.Contains(r.raw, "when:") {
		t.Fatalf("bad condition: %s", r.raw)
	}
	if r := add(`{"op": "add_transition", "field": "k_bad", "transition": {"name": "k_bad", "label": {"de": "x"}, "from": ["nowhere"], "to": "k_review"}}`, 422); !strings.Contains(r.raw, "not a state") {
		t.Fatalf("bad from: %s", r.raw)
	}
	// The live check of the editor.
	if r := call(t, admin, http.MethodPost, "/api/v1/types/probe.probe:check-condition", `{"when": "record.priority <= 2"}`, nil); r.code != 200 || r.body["ok"] != true {
		t.Fatalf("check ok: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/types/probe.probe:check-condition", `{"when": "record.priority <"}`, nil); r.code != 200 || r.body["ok"] != false || r.body["error"] == nil {
		t.Fatalf("check bad: %d %s", r.code, r.raw)
	}

	// A record with priority 2 goes new → in_progress → k_review; the condition keeps one
	// with priority 4 out.
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "ui26", "priority": 2}`, nil)
	if rec.code != 201 {
		t.Fatalf("record: %d %s", rec.code, rec.raw)
	}
	recID := rec.body["id"].(string)
	move := func(id, tr string, want int) resp {
		t.Helper()
		r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe/"+id+":transition", `{"transition": "`+tr+`"}`, nil)
		if r.code != want {
			t.Fatalf("%s: %d %s", tr, r.code, r.raw)
		}
		return r
	}
	move(recID, "start", 200)
	if r := move(recID, "k_review_it", 200); r.body["state"] != "k_review" {
		t.Fatalf("after k_review_it: %s", r.raw)
	}
	other := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "ui26 b", "priority": 4}`, nil)
	otherID := other.body["id"].(string)
	move(otherID, "start", 200)
	move(otherID, "k_review_it", 409)

	// A disabled transition of the package is neither offered nor taken.
	add(`{"op": "disable_transition", "field": "finish"}`, 200)
	offered := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe/"+otherID+"/transitions", "", nil)
	var infos []map[string]any
	if err := json.Unmarshal([]byte(offered.raw), &infos); err != nil || len(infos) == 0 {
		t.Fatalf("transitions: %d %s", offered.code, offered.raw)
	}
	for _, it := range infos {
		if it["name"] == "finish" {
			t.Fatalf("finish offered while disabled: %s", offered.raw)
		}
	}
	if r := move(otherID, "finish", 422); !strings.Contains(r.raw, "disabled") {
		t.Fatalf("finish while disabled: %s", r.raw)
	}
	add(`{"op": "enable_transition", "field": "finish"}`, 200)
	move(otherID, "finish", 200)

	// The state with a record in it stays; empty, it goes once nothing uses it.
	if r := add(`{"op": "remove_state", "field": "k_review"}`, 422); !strings.Contains(r.raw, "stand in state") {
		t.Fatalf("remove used state: %s", r.raw)
	}
	move(recID, "k_back", 200)
	if r := add(`{"op": "remove_state", "field": "k_review"}`, 422); !strings.Contains(r.raw, "still uses") {
		t.Fatalf("remove referenced state: %s", r.raw)
	}
	add(`{"op": "remove_transition", "field": "k_review_it"}`, 200)
	add(`{"op": "remove_transition", "field": "k_back"}`, 200)
	add(`{"op": "remove_state", "field": "k_review"}`, 200)
	typ := call(t, admin, http.MethodGet, "/api/v1/types/probe.probe", "", nil)
	for _, st := range typ.body["states"].([]any) {
		if st.(map[string]any)["name"] == "k_review" {
			t.Fatalf("k_review still there: %s", typ.raw)
		}
	}
}
