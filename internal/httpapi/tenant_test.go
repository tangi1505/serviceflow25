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
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/notify/smtptest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/tenant"
)

// tenantData is what one tenant holds in the test: a record of every type, a node below the
// tenant and an assignment.
type tenantData struct {
	path       string
	records    map[string]string // type → id
	node       string
	assignment string
}

// ids are the identifiers that must not show up in the other tenant.
func (d tenantData) ids() []string {
	out := []string{d.path, d.assignment}
	for _, id := range d.records {
		out = append(out, id)
	}
	return out
}

// tenantUser makes an account bound to the tenant, with the role admin at the tenant node,
// and signs it in. The administrator of the installation switches into the tenant for it.
func tenantUser(t *testing.T, admin *testAPI, name, path string) (*testAPI, string) {
	t.Helper()
	ctx := t.Context()
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/context", `{"tenant": "`+path+`"}`, nil); r.code != 200 {
		t.Fatalf("switch to %s: %d %s", path, r.code, r.raw)
	}
	acc, err := admin.auth.CreateAccount(ctx, name, name, testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "`+name+`"}`, nil)
	if p.code != 201 || p.body["org_path"] != path {
		t.Fatalf("person in %s: %d %s", path, p.code, p.raw)
	}
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	if err := tenant.Bind(ctx, admin.pool, name, path); err != nil {
		t.Fatal(err)
	}
	a := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "admin", "person_id": "`+pid.String()+`"}`, nil)
	if a.code != 201 || a.body["org_path"] != path {
		t.Fatalf("assign in %s: %d %s", path, a.code, a.raw)
	}
	admin.authz.Invalidate(ctx)
	in := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	if in.code != 200 || in.body["tenant"] != path {
		t.Fatalf("login %s: %d %s", name, in.code, in.raw)
	}
	return &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, in.hdr)}, a.body["id"].(string)
}

// The acceptance of PL44 and PL45: an installation turns into tenants; two tenants with an
// account each holding the role admin (scope all) see nothing of each other over any route, in
// list, search, single read and export (404), the event stream, jobs, logs, webhooks, clients
// and directories included; an administrator of the installation sees one tenant at a time and
// switches; roles and types stay with the installation.
func TestTenantBoundaryOverHTTP(t *testing.T) {
	// A database of its own: tenants change what every other test sees.
	if base := os.Getenv("ITSM_TEST_DATABASE_URL"); base != "" {
		t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_tenant"))
	}
	admin := apiServer(t)
	ctx := t.Context()
	types := installAllPackages(t, admin)

	// Without tenants the root is the only one, as before.
	if r := call(t, admin, http.MethodGet, "/api/v1/auth/me", "", nil); r.body["tenant"] != "root" || r.body["tenants"] != nil {
		t.Fatalf("me without tenants: %s", r.raw)
	}
	before := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "vor den Mandanten"}`, nil)
	if before.code != 201 || before.body["org_path"] != "root" {
		t.Fatalf("record before: %d %s", before.code, before.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "alt"}`, nil); r.code != 201 {
		t.Fatalf("node before: %d %s", r.code, r.raw)
	}

	// The first tenant adopts all that is there; the administrator stays with the installation.
	res, err := tenant.Create(ctx, admin.pool, tenant.Options{Name: "ta", Label: map[string]string{"de": "Mandant A", "en": "Tenant A"}, Adopt: true, Admins: []string{"apitest"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Nodes < 1 || res.Records["probe.probe"] != 1 || res.Records["core.person"] < 1 {
		t.Fatalf("adopted: %+v", res)
	}
	if _, err := tenant.Create(ctx, admin.pool, tenant.Options{Name: "tb", Adopt: true}); err == nil {
		t.Fatal("a second tenant adopts")
	}
	if _, err := tenant.Create(ctx, admin.pool, tenant.Options{Name: "tb", Label: map[string]string{"de": "Mandant B"}}); err != nil {
		t.Fatal(err)
	}
	admin.authz.Invalidate(ctx)
	me := call(t, admin, http.MethodGet, "/api/v1/auth/me", "", nil)
	if me.body["tenant"] != "root.ta" || !strings.Contains(me.raw, `"label":{"de":"Mandant B"}`) {
		t.Fatalf("me with tenants: %s", me.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe/"+before.body["id"].(string), "", nil); r.code != 200 || r.body["org_path"] != "root.ta" {
		t.Fatalf("the record before is in the first tenant: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/org/nodes/root.ta.alt", "", nil); r.code != 200 {
		t.Fatalf("the node before is in the first tenant: %d %s", r.code, r.raw)
	}
	// Below the root there are only tenants now.
	if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "neu"}`, nil); r.code != 422 || !strings.Contains(r.raw, "tenants") {
		t.Fatalf("node below the root: %d %s", r.code, r.raw)
	}

	userA, assignA := tenantUser(t, admin, "apitest_ta", "root.ta")
	userB, assignB := tenantUser(t, admin, "apitest_tb", "root.tb")
	a := tenantData{path: "root.ta", node: "root.ta.unit", assignment: assignA, records: map[string]string{}}
	b := tenantData{path: "root.tb", node: "root.tb.unit", assignment: assignB, records: map[string]string{}}
	for _, d := range []struct {
		api  *testAPI
		data *tenantData
	}{{userA, &a}, {userB, &b}} {
		if r := call(t, d.api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "`+d.data.path+`", "name": "unit"}`, nil); r.code != 201 {
			t.Fatalf("node in %s: %d %s", d.data.path, r.code, r.raw)
		}
		if r := call(t, d.api, http.MethodPut, "/api/v1/org/nodes/"+d.data.node+"/settings/apitest.tenant", `"geheim"`, nil); r.code != 204 {
			t.Fatalf("setting in %s: %d %s", d.data.path, r.code, r.raw)
		}
		s := &sampler{t: t, api: d.api, types: types, made: map[string]string{}, busy: map[string]bool{}}
		for _, name := range sortedKeys(types) {
			s.record(name)
		}
		d.data.records = s.made
		// Every record lands in the tenant without saying so.
		for name, id := range s.made {
			r := call(t, d.api, http.MethodGet, "/api/v1/records/"+name+"/"+id, "", nil)
			if p, _ := r.body["org_path"].(string); r.code != 200 || !strings.HasPrefix(p, d.data.path) {
				t.Fatalf("%s of %s: %d %s", name, d.data.path, r.code, r.raw)
			}
		}
	}

	// Every route, with the identifiers of the other tenant.
	tenantWalk(t, userB, a, types)
	tenantWalk(t, userA, b, types)
	// Nothing of A changed.
	for name, id := range a.records {
		r := call(t, userA, http.MethodGet, "/api/v1/records/"+name+"/"+id, "", nil)
		if r.code != 200 || r.body["version"] != 1.0 {
			t.Errorf("%s of A after the walk of B: %d %s", name, r.code, r.raw)
		}
	}
	if r := call(t, userA, http.MethodGet, "/api/v1/org/nodes/root.ta.unit/settings", "", nil); !strings.Contains(r.raw, "geheim") {
		t.Errorf("setting of A after the walk of B: %d %s", r.code, r.raw)
	}

	tenantSideData(t, admin, userA, userB, a, b)

	// A tenant account cannot leave its tenant, change roles or types, or create at the root.
	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/api/v1/auth/context", `{"tenant": "root.tb"}`, 422},
		{http.MethodPost, "/api/v1/roles", `{"name": "pl44_role", "label": {"de": "x"}}`, 403},
		{http.MethodPost, "/api/v1/types/probe.probe/customer", `{"version": 0, "change": {"op": "hide_field", "field": "priority"}}`, 403},
		{http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "x"}`, 404},
		{http.MethodPut, "/api/v1/org/nodes/root/settings/apitest.tenant", `1`, 404},
		{http.MethodPost, "/api/v1/assignments", `{"role": "admin", "person_id": "` + b.records["core.person"] + `", "org_path": "root.ta"}`, 404},
		{http.MethodPost, "/api/v1/assignments", `{"role": "admin", "person_id": "` + a.records["core.person"] + `", "org_path": "root.tb"}`, 404},
		{http.MethodPost, "/api/v1/records/probe.probe", `{"title": "fremd", "org_path": "root.tb"}`, 403},
	} {
		if r := call(t, userA, c.method, c.path, c.body, nil); r.code != c.want {
			t.Errorf("%s %s as A: %d %s, want %d", c.method, c.path, r.code, r.raw, c.want)
		}
	}

	// The administrator sees one tenant at a time and switches; the switch is in the security
	// log and the next session starts there.
	list := func(api *testAPI) string {
		return call(t, api, http.MethodGet, "/api/v1/records/probe.probe?limit=500", "", nil).raw
	}
	// tenantUser left the administrator in B.
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/context", `{"tenant": "root.ta"}`, nil); r.code != 200 || r.body["tenant"] != "root.ta" {
		t.Fatalf("switch to A: %d %s", r.code, r.raw)
	}
	if l := list(admin); !strings.Contains(l, a.records["probe.probe"]) || strings.Contains(l, b.records["probe.probe"]) {
		t.Fatalf("administrator in A: %s", l)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/context", `{"tenant": "root.tb"}`, nil); r.code != 200 || r.body["tenant"] != "root.tb" {
		t.Fatalf("switch: %d %s", r.code, r.raw)
	}
	if l := list(admin); strings.Contains(l, a.records["probe.probe"]) || !strings.Contains(l, b.records["probe.probe"]) {
		t.Fatalf("administrator in B: %s", l)
	}
	var switches int
	if err := admin.pool.QueryRow(ctx, "SELECT count(*) FROM security_event WHERE kind = 'tenant' AND details->>'to' = 'root.tb'").Scan(&switches); err != nil || switches == 0 {
		t.Fatalf("switch in the security log: %d %v", switches, err)
	}
	again := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	if again.code != 200 || again.body["tenant"] != "root.tb" {
		t.Fatalf("the next session starts in the last tenant: %d %s", again.code, again.raw)
	}
	// Roles belong to the installation: the administrator changes them in any tenant.
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "pl44_role", "label": {"de": "x"}}`, nil); r.code != 201 {
		t.Fatalf("role by the administrator: %d %s", r.code, r.raw)
	}
}

// tenantWalk calls every operation of the API as viewer with the identifiers of the other
// tenant: no answer may show one of them, a read of one is 404, and nothing else succeeds
// (PL44, PL45). Records go through every type.
func tenantWalk(t *testing.T, viewer *testAPI, other tenantData, types map[string]meta.Type) {
	t.Helper()
	doc, err := gen.GetSpec()
	if err != nil {
		t.Fatal(err)
	}
	ids := other.ids()
	var templates []string
	for p := range doc.Paths.Map() {
		templates = append(templates, p)
	}
	sort.Strings(templates)
	for _, tpl := range templates {
		if skipInWalk(tpl) {
			continue
		}
		for method, op := range doc.Paths.Map()[tpl].Operations() {
			// Required query arguments get a value, so that the route reaches the lookup.
			var query []string
			for _, p := range op.Parameters {
				if p.Value != nil && p.Value.In == "query" && p.Value.Required {
					query = append(query, p.Value.Name+"=1")
				}
			}
			typeNames := []string{"probe.probe"}
			if strings.HasPrefix(tpl, "/api/v1/records/{typeName}") {
				typeNames = sortedKeys(types)
			}
			for _, typeName := range typeNames {
				path, foreign := fillTemplate(tpl, typeName, other)
				if len(query) > 0 {
					path += "?" + strings.Join(query, "&")
				}
				body := ""
				if method != http.MethodGet && method != http.MethodDelete {
					body = "{}"
				}
				r := call(t, viewer, method, path, body, nil)
				leak := ""
				for _, id := range ids {
					// An identifier the route names may come back in the message of a 404.
					if id != "" && !strings.Contains(path, id) && strings.Contains(r.raw, id) {
						leak = id
					}
				}
				bad := ""
				switch {
				case leak != "":
					bad = "shows " + leak
				case foreign && method == http.MethodGet && r.code != http.StatusNotFound:
					bad = fmt.Sprintf("answers %d, not 404", r.code)
				case foreign && (r.code < 300 || r.code == http.StatusForbidden || r.code == http.StatusConflict):
					bad = fmt.Sprintf("answers %d", r.code)
				}
				if bad == "" {
					continue
				}
				msg := fmt.Sprintf("%s %s %s: %s", method, path, bad, strings.TrimSpace(r.raw))
				if len(msg) > 400 {
					msg = msg[:400]
				}
				t.Errorf("%s", msg)
			}
		}
	}
}

// skipInWalk are the operations of signing in and out: they hold no data of a tenant and
// would end the session of the walk; the event stream does not end.
func skipInWalk(tpl string) bool {
	switch tpl {
	case "/api/v1/auth/login", "/api/v1/auth/logout", "/api/v1/auth/password", "/api/v1/auth/context",
		"/api/v1/auth/token", "/api/v1/auth/oidc/start", "/api/v1/auth/oidc/callback", "/api/v1/events/stream":
		return true
	}
	return false
}

// fillTemplate puts identifiers of the other tenant into a route; foreign says whether one of
// them names something of that tenant.
func fillTemplate(tpl, typeName string, other tenantData) (string, bool) {
	foreign := false
	out := tpl
	for {
		i := strings.Index(out, "{")
		if i < 0 {
			return out, foreign
		}
		j := strings.Index(out[i:], "}") + i
		name := out[i+1 : j]
		var v string
		switch name {
		case "typeName":
			v = typeName
		case "id":
			switch {
			case strings.HasPrefix(tpl, "/api/v1/records/"):
				v, foreign = other.records[typeName], true
			case strings.HasPrefix(tpl, "/api/v1/groups/"):
				v, foreign = other.records["core.group"], true
			case strings.HasPrefix(tpl, "/api/v1/assignments/"):
				v, foreign = other.assignment, true
			default:
				v = uuid.NewString()
			}
		case "formId":
			v, foreign = other.records["core.form"], true
		case "calendarId":
			v, foreign = other.records["core.calendar"], true
		case "path":
			v, foreign = other.node, true
		case "key":
			v = "apitest.tenant"
		case "name":
			v = "admin"
		case "action":
			v = "read"
		case "n":
			v = "1"
		default:
			v = uuid.NewString()
		}
		out = out[:i] + v + out[j+1:]
	}
}

// tenantSideData is the acceptance of PL45: select lists are copies per tenant; rules,
// notifications and webhooks of one tenant do not act on events of the other, those of the
// installation act in both; an API client of A gets nothing of B; the security log of B
// shows no switch of the administrator.
func tenantSideData(t *testing.T, admin, userA, userB *testAPI, a, b tenantData) {
	t.Helper()
	ctx := t.Context()

	// Select lists: each tenant has its own copy, tb got it when it was made (V28).
	reds := map[string]string{}
	for path, u := range map[string]*testAPI{a.path: userA, b.path: userB} {
		r := call(t, u, http.MethodGet, "/api/v1/records/probe.color?limit=500", "", nil)
		items, _ := r.body["items"].([]any)
		for _, it := range items {
			if e := it.(map[string]any); e["key"] == "red" {
				if reds[path] != "" || e["org_path"] != path {
					t.Fatalf("red in %s: %s", path, r.raw)
				}
				reds[path] = e["id"].(string)
			}
		}
		if r.code != 200 || reds[path] == "" {
			t.Fatalf("red in %s: %d %s", path, r.code, r.raw)
		}
	}
	if reds[a.path] == reds[b.path] {
		t.Fatal("both tenants share one list entry")
	}

	create := func(api *testAPI, typeName, body string) map[string]any {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body
	}

	// Rules: one of B acts in B only; the same name in A is another rule; one of the
	// installation acts in both and only an administrator of the installation makes it.
	rule := func(api *testAPI, name, body string, want int) {
		t.Helper()
		if r := call(t, api, http.MethodPut, "/api/v1/rules/"+name, body, nil); r.code != want {
			t.Fatalf("rule %s: %d %s", name, r.code, r.raw)
		}
	}
	rule(userB, "pl45_prio", `{"type_name": "probe.probe", "trigger": "create", "mode": "before", "actions": [{"set": {"priority": 5}}]}`, 201)
	if r := call(t, userA, http.MethodGet, "/api/v1/rules/pl45_prio", "", nil); r.code != 404 {
		t.Fatalf("rule of B in A: %d %s", r.code, r.raw)
	}
	rule(userA, "pl45_prio", `{"type_name": "probe.probe", "trigger": "create", "mode": "before", "actions": [{"set": {"priority": 2}}]}`, 201)
	rule(userA, "pl45_all", `{"installation": true, "type_name": "probe.probe", "trigger": "create", "mode": "before", "actions": [{"set": {"solution": "Installation"}}]}`, 403)
	rule(admin, "pl45_all", `{"installation": true, "type_name": "probe.probe", "trigger": "create", "mode": "before", "actions": [{"set": {"solution": "Installation"}}]}`, 201)
	if r := call(t, userB, http.MethodPut, "/api/v1/rules/pl45_all", `{"type_name": "probe.probe", "trigger": "create", "mode": "before", "actions": []}`, nil); r.code != 403 {
		t.Fatalf("B changes a rule of the installation: %d %s", r.code, r.raw)
	}
	pa := create(userA, "probe.probe", `{"title": "PL45 Regel A", "priority": 1}`)
	pb := create(userB, "probe.probe", `{"title": "PL45 Regel B", "priority": 1}`)
	if pa["priority"] != 2.0 || pb["priority"] != 5.0 || pa["solution"] != "Installation" || pb["solution"] != "Installation" {
		t.Fatalf("rules: A %v/%v, B %v/%v", pa["priority"], pa["solution"], pb["priority"], pb["solution"])
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/rules/pl45_all", "", nil); r.code != 204 {
		t.Fatalf("delete the rule of the installation: %d %s", r.code, r.raw)
	}

	// Webhooks: the subscription of B gets no event of A, the one of A gets it.
	hook := func(api *testAPI, name string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/webhooks", `{"name": "`+name+`", "url": "https://receiver.example/hook"}`, nil)
		if r.code != 201 {
			t.Fatalf("webhook %s: %d %s", name, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	hookA, hookB := hook(userA, "pl45 a"), hook(userB, "pl45 b")
	rec := create(userA, "probe.probe", `{"title": "PL45 Webhook"}`)
	id := uuid.MustParse(rec["id"].(string))
	var e records.Event
	for deadline := time.Now().Add(10 * time.Second); e.Position == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		list, err := records.New(admin.pool).Events(ctx, 0, 100000)
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range list {
			if ev.RecordID == id {
				e = ev
			}
		}
	}
	if err := admin.hooks.Handle(ctx, e); err != nil {
		t.Fatal(err)
	}
	deliveries := func(api *testAPI, hook string) int {
		t.Helper()
		r := call(t, api, http.MethodGet, "/api/v1/webhooks/"+hook+"/deliveries", "", nil)
		var list []map[string]any
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil {
			t.Fatalf("deliveries: %d %s", r.code, r.raw)
		}
		return len(list)
	}
	if da, db := deliveries(userA, hookA), deliveries(userB, hookB); da != 1 || db != 0 {
		t.Fatalf("deliveries: A %d, B %d", da, db)
	}

	// Notifications: one of B mails for its records only; one of A that names a person of B
	// sends nothing to that person.
	personB := create(userB, "core.person", `{"last_name": "Empfänger B", "email": "pl45-b@example.org"}`)["id"].(string)
	create(userB, "core.notification", `{"name": "PL45 B", "target_type": "probe.probe", "events": "create",
		"recipients": "core.person:`+personB+`", "subject": "PL45 {{record.title}}", "body": "x"}`)
	if r := call(t, userA, http.MethodPost, "/api/v1/records/core.notification", `{"name": "PL45 fremd", "target_type": "probe.probe",
		"events": "create", "recipients": "core.person:`+personB+`", "subject": "PL45 {{record.title}}", "body": "x"}`, nil); r.code != 201 && r.code != 422 {
		t.Fatalf("notification of A naming B: %d %s", r.code, r.raw)
	}
	create(userA, "probe.probe", `{"title": "aus A"}`)
	create(userB, "probe.probe", `{"title": "aus B"}`)
	mails := waitForMail(t, admin, 1, func(m smtptest.Mail) bool { return strings.HasPrefix(m.Subject, "PL45 aus") })
	if mails[0].Subject != "PL45 aus B" || mails[0].To[0] != "pl45-b@example.org" {
		t.Fatalf("mail: %+v", mails[0])
	}

	// An API client of A: its account belongs to A, B does not see it, and its token reads A only.
	c := call(t, userA, http.MethodPost, "/api/v1/clients", `{"client_id": "pl45_a"}`, nil)
	if c.code != 201 {
		t.Fatalf("client: %d %s", c.code, c.raw)
	}
	if r := call(t, userA, http.MethodPost, "/api/v1/assignments", `{"role": "admin", "person_id": "`+c.body["person_id"].(string)+`"}`, nil); r.code != 201 {
		t.Fatalf("role of the client: %d %s", r.code, r.raw)
	}
	if r := call(t, userB, http.MethodGet, "/api/v1/clients/pl45_a", "", nil); r.code != 404 {
		t.Fatalf("client of A in B: %d %s", r.code, r.raw)
	}
	admin.authz.Invalidate(ctx)
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {"pl45_a"}, "client_secret": {c.body["secret"].(string)}}
	tok := call(t, &testAPI{Server: admin.Server}, http.MethodPost, "/api/v1/auth/token", form.Encode(),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if tok.code != 200 {
		t.Fatalf("token: %d %s", tok.code, tok.raw)
	}
	bearer := map[string]string{"Authorization": "Bearer " + tok.body["access_token"].(string)}
	list := call(t, &testAPI{Server: admin.Server}, http.MethodGet, "/api/v1/records/probe.probe?limit=500", "", bearer)
	if list.code != 200 || !strings.Contains(list.raw, a.records["probe.probe"]) || strings.Contains(list.raw, b.records["probe.probe"]) {
		t.Fatalf("client of A reads: %d %.300s", list.code, list.raw)
	}

	// The security log: B sees its own entries only, not the switches of the administrator.
	for _, u := range []*testAPI{userB, admin} {
		r := call(t, u, http.MethodGet, "/api/v1/audit/auth?kind=tenant", "", nil)
		items, _ := r.body["items"].([]any)
		if r.code != 200 || (u == userB) != (len(items) == 0) {
			t.Fatalf("switches in the log: %d %s", r.code, r.raw)
		}
	}
}
