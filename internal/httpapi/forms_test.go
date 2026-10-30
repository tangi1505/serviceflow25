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
	"strings"
	"testing"
	"time"
)

// formDefinition is a form for probe.probe with three sections: the title and the kind bound
// to their fields, then free questions of each kind of answer.
const formDefinition = `{"sections": [
  {"id": "s1", "title": {"de": "Anliegen", "en": "Request"}, "questions": [
    {"id": "q_title", "field": "title", "label": {"de": "Worum geht es?"}, "required": true},
    {"id": "q_kind", "field": "kind"}]},
  {"id": "s2", "title": {"de": "Ort"}, "questions": [
    {"id": "q_room", "type": "text", "label": {"de": "Raum"}, "required": true},
    {"id": "q_floor", "type": "choice", "label": {"de": "Etage"}, "options": [
      {"value": "eg", "label": {"de": "Erdgeschoss"}}, {"value": "og1", "label": {"de": "1. OG"}}]}]},
  {"id": "s3", "title": {"de": "Dringlichkeit"}, "questions": [
    {"id": "q_urgent", "type": "boolean", "label": {"de": "Dringend"}},
    {"id": "q_people", "type": "number", "label": {"de": "Betroffene"}}]}]}`

// The acceptance of UI11 over HTTP: the definition is checked when saving; an active form is
// offered to whoever may create its target; submitting creates the record with the requester
// and the free answers as its first entry, in one go, without the right comment.
func TestFormsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	perms := map[string][]string{
		"ui11_user": {`{"type_name": "probe.probe", "action": "read", "scope": "own"}`,
			`{"type_name": "probe.probe", "action": "read", "scope": "involved"}`,
			`{"type_name": "probe.probe", "action": "create", "scope": "all"}`},
		"ui11_reader": {`{"type_name": "probe.probe", "action": "read", "scope": "all"}`},
	}
	for role, list := range perms {
		call(t, admin, http.MethodDelete, "/api/v1/roles/"+role, "", nil)
		if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "`+role+`"}}`, nil); r.code != 201 {
			t.Fatalf("role %s: %d %s", role, r.code, r.raw)
		}
		for _, p := range list {
			if r := call(t, admin, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
				t.Fatalf("permission %s: %d %s", role, r.code, r.raw)
			}
		}
	}
	user := timelineLogin(t, admin, "apitest_ui11_user", "ui11_user", `{"last_name": "UI11"}`)
	reader := timelineLogin(t, admin, "apitest_ui11_reader", "ui11_reader", `{"last_name": "UI11 Leser"}`)

	// Saving checks the definition against the target type.
	for _, c := range []struct{ name, definition, want string }{
		{"not json", `sections`, "JSON of a form"},
		{"unknown field", `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "x", "field": "nope"}]}]}`, `no field \"nope\"`},
		{"calculated", `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "x", "field": "tasks_open"}]}]}`, "calculated"},
		{"title missing", `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "x", "type": "text", "label": {"de": "Frei"}}]}]}`, "needs title"},
		{"twice", `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "q", "field": "kind"}]}]}`, "id of its own"},
		{"choice without options", `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "c", "type": "choice", "label": {"de": "C"}}]}]}`, "needs options"},
		{"reference free", `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}, {"id": "r", "type": "reference", "label": {"de": "R"}}]}]}`, "must be text"},
	} {
		body, _ := json.Marshal(map[string]any{"name": "falsch", "target_type": "probe.probe", "definition": c.definition})
		r := call(t, admin, http.MethodPost, "/api/v1/records/core.form", string(body), nil)
		if r.code != 422 || !strings.Contains(r.raw, c.want) {
			t.Fatalf("%s: %d %s", c.name, r.code, r.raw)
		}
	}

	suffix := fmt.Sprint(time.Now().UnixNano())
	body, _ := json.Marshal(map[string]any{"name": "Störung melden " + suffix, "target_type": "probe.probe", "active": true, "definition": formDefinition})
	f := call(t, admin, http.MethodPost, "/api/v1/records/core.form", string(body), nil)
	if f.code != 201 {
		t.Fatalf("form: %d %s", f.code, f.raw)
	}
	id := f.body["id"].(string)
	body, _ = json.Marshal(map[string]any{"name": "Entwurf " + suffix, "target_type": "probe.probe", "definition": formDefinition})
	draft := call(t, admin, http.MethodPost, "/api/v1/records/core.form", string(body), nil)
	if draft.code != 201 {
		t.Fatalf("draft: %d %s", draft.code, draft.raw)
	}

	// Offered to whoever may create probe.probe; a draft to nobody; reading core.form is not needed.
	listed := func(api *testAPI) map[string]bool {
		t.Helper()
		r := call(t, api, http.MethodGet, "/api/v1/forms", "", nil)
		if r.code != 200 {
			t.Fatalf("list: %d %s", r.code, r.raw)
		}
		var list []map[string]any
		_ = json.Unmarshal([]byte(r.raw), &list)
		out := map[string]bool{}
		for _, x := range list {
			out[x["id"].(string)] = true
		}
		return out
	}
	if l := listed(user.api); !l[id] || l[draft.body["id"].(string)] {
		t.Fatalf("user sees %v", l)
	}
	if l := listed(reader.api); l[id] {
		t.Fatalf("a reader without create sees the form")
	}
	if r := call(t, reader.api, http.MethodGet, "/api/v1/forms/"+id, "", nil); r.code != 404 {
		t.Fatalf("reader gets the form: %d", r.code)
	}
	if r := call(t, user.api, http.MethodGet, "/api/v1/records/core.form/"+id, "", nil); r.code != 403 && r.code != 404 {
		t.Fatalf("the record of the form stays closed: %d", r.code)
	}
	if r := call(t, user.api, http.MethodGet, "/api/v1/forms/"+id, "", nil); r.code != 200 || !strings.Contains(r.raw, `"q_room"`) {
		t.Fatalf("user gets the form: %d %s", r.code, r.raw)
	}

	// Answers are checked per question.
	submit := "/api/v1/forms/" + id + ":submit"
	r := call(t, user.api, http.MethodPost, submit, `{"answers": {"q_floor": "keller", "q_urgent": "ja", "q_extra": 1}}`, nil)
	for _, want := range []string{`"field":"q_title"`, `"field":"q_room"`, `"field":"q_floor"`, `"field":"q_urgent"`, `"field":"q_extra"`} {
		if r.code != 422 || !strings.Contains(r.raw, want) {
			t.Fatalf("problems must name %s: %d %s", want, r.code, r.raw)
		}
	}
	r = call(t, user.api, http.MethodPost, submit, `{"answers": {"q_title": "`+strings.Repeat("x", 300)+`", "q_room": "2.14"}}`, nil)
	if r.code != 422 || !strings.Contains(r.raw, `"field":"q_title"`) {
		t.Fatalf("a problem of the record names its question: %d %s", r.code, r.raw)
	}

	r = call(t, user.api, http.MethodPost, submit, `{"answers": {"q_title": "Drucker `+suffix+`", "q_kind": "fault",
		"q_room": "2.14", "q_floor": "og1", "q_urgent": true, "q_people": 3}}`, nil)
	if r.code != 201 || r.body["title"] != "Drucker "+suffix || r.body["kind"] != "fault" {
		t.Fatalf("submit: %d %s", r.code, r.raw)
	}
	rid := r.body["id"].(string)
	if !strings.Contains(r.raw, `"requester":[{"id":"`+user.person+`","type":"core.person"}]`) {
		t.Fatalf("the person is the requester: %s", r.raw)
	}
	// The user reads the record as its requester; the free answers are its first entry.
	entries := entryBodies(timeline(t, user.api, "/api/v1/records/probe.probe/"+rid))
	want := "Ort\nRaum: 2.14\nEtage: 1. OG\n\nDringlichkeit\nDringend: ja\nBetroffene: 3"
	if len(entries) != 1 || entries[0] != want {
		t.Fatalf("first entry: %q", entries)
	}
	// One free answer: an entry of one section and one line.
	r = call(t, user.api, http.MethodPost, submit, `{"answers": {"q_title": "Nur Titel `+suffix+`", "q_room": "1"}}`, nil)
	if r.code != 201 {
		t.Fatalf("submit: %d %s", r.code, r.raw)
	}
	if e := entryBodies(timeline(t, user.api, "/api/v1/records/probe.probe/"+r.body["id"].(string))); len(e) != 1 || e[0] != "Ort\nRaum: 1" {
		t.Fatalf("entry with one answer: %q", e)
	}
	// A reader may not submit.
	if r := call(t, reader.api, http.MethodPost, submit, `{"answers": {"q_title": "x", "q_room": "1"}}`, nil); r.code != 404 {
		t.Fatalf("reader submits: %d %s", r.code, r.raw)
	}
	// Taken back from the portal, the form is gone for everyone.
	if r := call(t, admin, http.MethodPatch, "/api/v1/records/core.form/"+id, `{"active": false}`, map[string]string{"If-Match": `"1"`}); r.code != 200 {
		t.Fatalf("deactivate: %d %s", r.code, r.raw)
	}
	if r := call(t, user.api, http.MethodPost, submit, `{"answers": {"q_title": "x", "q_room": "1"}}`, nil); r.code != 404 {
		t.Fatalf("inactive form submitted: %d %s", r.code, r.raw)
	}
}

// The acceptance of UI12 over HTTP: question B shows only when A is "yes"; hidden, its answer
// is dropped and "required" does not apply; shown, it is required. A condition on a later
// question is refused when saving. The entry is German, the English texts travel with the form.
func TestFormConditionsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	call(t, admin, http.MethodDelete, "/api/v1/roles/ui12_user", "", nil)
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "ui12_user", "label": {"de": "ui12"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, p := range []string{`{"type_name": "probe.probe", "action": "read", "scope": "involved"}`, `{"type_name": "probe.probe", "action": "create", "scope": "all"}`} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/ui12_user/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	user := timelineLogin(t, admin, "apitest_ui12_user", "ui12_user", `{"last_name": "UI12"}`)

	later := `{"sections": [{"id": "s1", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title", "show_if": [{"question": "b", "op": "answered"}]},
	  {"id": "b", "type": "text", "label": {"de": "B"}}]}]}`
	body, _ := json.Marshal(map[string]any{"name": "falsch", "target_type": "probe.probe", "definition": later})
	if r := call(t, admin, http.MethodPost, "/api/v1/records/core.form", string(body), nil); r.code != 422 || !strings.Contains(r.raw, "not a question before") {
		t.Fatalf("condition on a later question: %d %s", r.code, r.raw)
	}

	definition := `{"title": {"en": "Report a fault"}, "sections": [
	  {"id": "s1", "title": {"de": "Anliegen", "en": "Request"}, "questions": [
	    {"id": "q_title", "field": "title"},
	    {"id": "q_spare", "type": "boolean", "label": {"de": "Ersatzgerät nötig?", "en": "Spare device needed?"}},
	    {"id": "q_which", "type": "text", "label": {"de": "Welches Gerät?"}, "required": true,
	     "show_if": [{"question": "q_spare", "op": "eq", "value": "true"}]}]},
	  {"id": "s2", "title": {"de": "Lieferung"}, "show_if": [{"question": "q_spare", "op": "eq", "value": "true"}], "questions": [
	    {"id": "q_room", "type": "text", "label": {"de": "Raum"}}]}]}`
	body, _ = json.Marshal(map[string]any{"name": fmt.Sprint("Bedingt ", time.Now().UnixNano()), "target_type": "probe.probe", "active": true, "definition": definition})
	f := call(t, admin, http.MethodPost, "/api/v1/records/core.form", string(body), nil)
	if f.code != 201 {
		t.Fatalf("form: %d %s", f.code, f.raw)
	}
	id := f.body["id"].(string)
	if r := call(t, user.api, http.MethodGet, "/api/v1/forms/"+id, "", nil); !strings.Contains(r.raw, `"title":{"en":"Report a fault"}`) || !strings.Contains(r.raw, `"Spare device needed?"`) {
		t.Fatalf("the English texts: %s", r.raw)
	}
	submit := "/api/v1/forms/" + id + ":submit"

	// "No": B and the section Lieferung are hidden; their answers are dropped, B is not required.
	r := call(t, user.api, http.MethodPost, submit, `{"answers": {"q_title": "Ohne Ersatz", "q_spare": false, "q_which": "Laptop", "q_room": "1.01"}}`, nil)
	if r.code != 201 {
		t.Fatalf("no: %d %s", r.code, r.raw)
	}
	if e := entryBodies(timeline(t, user.api, "/api/v1/records/probe.probe/"+r.body["id"].(string))); len(e) != 1 || e[0] != "Anliegen\nErsatzgerät nötig?: nein" {
		t.Fatalf("hidden answers kept: %q", e)
	}
	// "Yes": B shows and is required.
	r = call(t, user.api, http.MethodPost, submit, `{"answers": {"q_title": "Mit Ersatz", "q_spare": true}}`, nil)
	if r.code != 422 || !strings.Contains(r.raw, `"field":"q_which"`) {
		t.Fatalf("yes without B: %d %s", r.code, r.raw)
	}
	r = call(t, user.api, http.MethodPost, submit, `{"answers": {"q_title": "Mit Ersatz", "q_spare": true, "q_which": "Laptop", "q_room": "1.01"}}`, nil)
	if r.code != 201 {
		t.Fatalf("yes: %d %s", r.code, r.raw)
	}
	want := "Anliegen\nErsatzgerät nötig?: ja\nWelches Gerät?: Laptop\n\nLieferung\nRaum: 1.01"
	if e := entryBodies(timeline(t, user.api, "/api/v1/records/probe.probe/"+r.body["id"].(string))); len(e) != 1 || e[0] != want {
		t.Fatalf("entry: %q", e)
	}
}

// A form belongs to its node (architektur.md §5.1, gap 4): it is offered to whoever may create
// its target there, not to anyone who may create it somewhere, and its record lands there.
func TestFormNodeOverHTTP(t *testing.T) {
	admin := apiServer(t)
	call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root.formsnode", "", nil)
	// A node from an earlier run that holds records stays; then it exists already.
	if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "formsnode"}`, nil); r.code != 201 && r.code != 409 {
		t.Fatalf("node: %d %s", r.code, r.raw)
	}
	for _, role := range []string{"ui11_unit", "ui11_none"} {
		call(t, admin, http.MethodDelete, "/api/v1/roles/"+role, "", nil)
		if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "`+role+`"}}`, nil); r.code != 201 {
			t.Fatalf("role: %d %s", r.code, r.raw)
		}
	}
	for _, p := range []string{`{"type_name": "probe.probe", "action": "read", "scope": "subtree"}`, `{"type_name": "probe.probe", "action": "create", "scope": "subtree"}`} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/ui11_unit/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	user := timelineLogin(t, admin, "apitest_ui11_unit", "ui11_none", `{"last_name": "Knoten"}`)
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "ui11_unit", "person_id": "`+user.person+`", "org_path": "root.formsnode"}`, nil); r.code != 201 {
		t.Fatalf("assign: %d %s", r.code, r.raw)
	}
	suffix := fmt.Sprint(time.Now().UnixNano())
	ids := map[string]string{}
	for name, node := range map[string]string{"Wurzel": "root", "Knoten": "root.formsnode"} {
		body, _ := json.Marshal(map[string]any{"name": name + " " + suffix, "target_type": "probe.probe", "active": true,
			"org_path": node, "definition": `{"sections": [{"id": "s", "title": {"de": "A"}, "questions": [{"id": "q", "field": "title"}]}]}`})
		r := call(t, admin, http.MethodPost, "/api/v1/records/core.form", string(body), nil)
		if r.code != 201 {
			t.Fatalf("form %s: %d %s", name, r.code, r.raw)
		}
		ids[name] = r.body["id"].(string)
	}
	var listed string
	for tries := 0; ; tries++ {
		listed = call(t, user.api, http.MethodGet, "/api/v1/forms", "", nil).raw
		if strings.Contains(listed, ids["Knoten"]) {
			break
		}
		if tries > 50 {
			t.Fatalf("the form of the node is not offered: %s", listed)
		}
	}
	if strings.Contains(listed, ids["Wurzel"]) {
		t.Fatalf("a form at the root is offered to a person who creates only below formsnode: %s", listed)
	}
	if r := call(t, user.api, http.MethodGet, "/api/v1/forms/"+ids["Wurzel"], "", nil); r.code != 404 {
		t.Fatalf("form at the root: %d", r.code)
	}
	r := call(t, user.api, http.MethodPost, "/api/v1/forms/"+ids["Knoten"]+":submit", `{"answers": {"q": "Im Knoten `+suffix+`"}}`, nil)
	if r.code != 201 || r.body["org_path"] != "root.formsnode" {
		t.Fatalf("submit: %d %s", r.code, r.raw)
	}
}
