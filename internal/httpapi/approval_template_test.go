// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of WF7 (WF-Plan E18): an active template appears filled in for a record, its
// role "actors.group.responsible" resolved to the responsible person of the record's group, a
// stage without anybody left out, an inactive template not at all. Asked from it, the approval
// names the template and its working days; the job reminds the open stage once its moment has
// come, counts it and sets the next one; a template with 0 days never reminds. A stage with a
// role that is none is refused, so is an unknown template.
func TestApprovalTemplatesOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_wf7"))
	admin := apiServer(t)
	ctx := t.Context()
	starter := seed.New(admin.rt, records.New(admin.rt), admin.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 28}, {"../../packages/request", 12}} {
		p := installPackage(t, ctx, admin.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "wf7_user", "label": {"de": "WF7"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	lead := timelineLogin(t, admin, "apitest_wf7_lead", "wf7_user", `{"last_name": "Leitung"}`)
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, admin, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	team := create("core.group", `{"name": "WF7 Team", "responsible": "`+lead.person+`"}`)
	other := create("core.person", `{"last_name": "Lizenz"}`)

	sw := create("core.approval_template", `{"name": "Software", "message": "Bitte Lizenz prüfen.", "remind_days": 1}`)
	create("core.approval_template_stage", `{"template": "`+sw+`", "name": "Fachlich", "position": 10, "roles": "actors.group.responsible"}`)
	create("core.approval_template_stage", `{"template": "`+sw+`", "name": "Lizenz", "position": 20,
		"actors": {"approvers": [{"type": "core.person", "id": "`+other+`"}]}}`)
	create("core.approval_template_stage", `{"template": "`+sw+`", "name": "Leer", "position": 30, "roles": "actors.observer"}`)
	old := create("core.approval_template", `{"name": "Alt", "active": false}`)
	create("core.approval_template_stage", `{"template": "`+old+`", "name": "Eins", "actors": {"approvers": [{"type": "core.person", "id": "`+other+`"}]}}`)
	quiet := create("core.approval_template", `{"name": "Ohne Erinnerung", "remind_days": 0}`)
	create("core.approval_template_stage", `{"template": "`+quiet+`", "name": "Eins", "actors": {"approvers": [{"type": "core.person", "id": "`+other+`"}]}}`)
	if r := call(t, admin, http.MethodPost, "/api/v1/records/core.approval_template_stage",
		`{"template": "`+sw+`", "name": "Falsch", "roles": "group"}`, nil); r.code != 422 {
		t.Fatalf("a role that is none: %d %s", r.code, r.raw)
	}

	const requests = "/api/v1/records/request.request"
	id := create("request.request", `{"title": "Acrobat", "actors": {"group": [{"type": "core.group", "id": "`+team+`"}]}}`)

	// The drafts: by name, the inactive one left out, the empty stage left out.
	r := call(t, admin, http.MethodGet, requests+"/"+id+"/approval-templates", "", nil)
	var drafts []records.ApprovalDraft
	if r.code != 200 || json.Unmarshal([]byte(r.raw), &drafts) != nil {
		t.Fatalf("drafts: %d %s", r.code, r.raw)
	}
	if len(drafts) != 2 || drafts[0].Name != "Ohne Erinnerung" || drafts[1].Name != "Software" {
		t.Fatalf("drafts: %s", r.raw)
	}
	d := drafts[1]
	if d.Message != "Bitte Lizenz prüfen." || d.RemindDays == nil || *d.RemindDays != 1 || len(d.Stages) != 2 ||
		d.Stages[0].Name != "Fachlich" || len(d.Stages[0].Approvers) != 1 || d.Stages[0].Approvers[0].ID.String() != lead.person ||
		d.Stages[0].Approvers[0].Role != "group.responsible" || d.Stages[1].Approvers[0].ID.String() != other || d.Stages[1].Approvers[0].Role != "" {
		t.Fatalf("draft: %s", r.raw)
	}
	if r := call(t, lead.api, http.MethodGet, requests+"/"+id+"/approval-templates", "", nil); r.code != 403 && r.code != 404 {
		t.Fatalf("drafts without the right to change: %d %s", r.code, r.raw)
	}

	// Asked from the draft: the approval names the template and its days.
	body, _ := json.Marshal(d.Request())
	r = call(t, admin, http.MethodPost, requests+"/"+id+"/approvals", string(body), nil)
	if r.code != 201 || r.body["template"] != sw || r.body["remind_days"] != float64(1) {
		t.Fatalf("ask from the template: %d %s", r.code, r.raw)
	}
	approval := r.body["id"].(string)
	stage := func(id string) map[string]any {
		t.Helper()
		r := call(t, admin, http.MethodGet, requests+"/"+id+"/approvals", "", nil)
		var list []map[string]any
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil || len(list) == 0 {
			t.Fatalf("approvals: %d %s", r.code, r.raw)
		}
		return list[0]["stages"].([]any)[0].(map[string]any)
	}
	first := stage(id)
	if first["remind_at"] == nil || first["reminders"] != float64(0) || first["reminded_at"] != nil {
		t.Fatalf("before the reminder: %v", first)
	}

	// The job: nothing due yet; after a day and a half the stage reminds once, then not again
	// at the same moment.
	rec := records.New(admin.rt)
	remind := func(at time.Time) int {
		t.Helper()
		n := 0
		for _, db := range admin.rt.Databases() {
			k, err := rec.RemindApprovals(db.Bind(ctx), at)
			if err != nil {
				t.Fatal(err)
			}
			n += k
		}
		return n
	}
	if n := remind(time.Now()); n != 0 {
		t.Fatalf("reminded too early: %d", n)
	}
	later := time.Now().Add(36 * time.Hour)
	if n := remind(later); n != 1 {
		t.Fatalf("reminders after a day and a half: %d", n)
	}
	after := stage(id)
	if after["reminders"] != float64(1) || after["reminded_at"] == nil || after["remind_at"] == first["remind_at"] {
		t.Fatalf("after the reminder: %v (before %v)", after, first)
	}
	if n := remind(later); n != 0 {
		t.Fatalf("reminded twice: %d", n)
	}

	// Withdrawn, the stage reminds no more; a template with 0 days never reminds.
	if r := call(t, admin, http.MethodPost, requests+"/"+id+"/approvals/"+approval+":withdraw", "", nil); r.code != 200 {
		t.Fatalf("withdraw: %d %s", r.code, r.raw)
	}
	if n := remind(time.Now().Add(30 * 24 * time.Hour)); n != 0 {
		t.Fatalf("a withdrawn approval reminded: %d", n)
	}
	silent := create("request.request", `{"title": "Ohne Erinnerung"}`)
	body, _ = json.Marshal(drafts[0].Request())
	if r := call(t, admin, http.MethodPost, requests+"/"+silent+"/approvals", string(body), nil); r.code != 201 || r.body["remind_days"] != float64(0) {
		t.Fatalf("ask without reminders: %d %s", r.code, r.raw)
	}
	if s := stage(silent); s["remind_at"] != nil {
		t.Fatalf("a stage without reminders: %v", s)
	}

	// An unknown template is a problem of the request.
	other2 := create("request.request", `{"title": "Zweites"}`)
	if r := call(t, admin, http.MethodPost, requests+"/"+other2+"/approvals",
		`{"template": "00000000-0000-0000-0000-000000000001", "stages": [{"approvers": [{"type": "core.person", "id": "`+other+`"}]}]}`, nil); r.code != 422 {
		t.Fatalf("an unknown template: %d %s", r.code, r.raw)
	}
}
