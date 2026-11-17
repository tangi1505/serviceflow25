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
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authn/authntest"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/branding"
	"group24.de/itsm/internal/bulk"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/calendar"
	"group24.de/itsm/internal/customer"
	"group24.de/itsm/internal/dashboard"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/export"
	"group24.de/itsm/internal/files"
	"group24.de/itsm/internal/forms"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/notify"
	"group24.de/itsm/internal/notify/smtptest"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/portal"
	"group24.de/itsm/internal/prefs"
	"group24.de/itsm/internal/presence"
	"group24.de/itsm/internal/query"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/recurrence"
	"group24.de/itsm/internal/relations"
	"group24.de/itsm/internal/rules/engine"
	"group24.de/itsm/internal/savedsearch"
	"group24.de/itsm/internal/schema"
	"group24.de/itsm/internal/search"
	"group24.de/itsm/internal/seclog"
	"group24.de/itsm/internal/settings"
	"group24.de/itsm/internal/sla"
	"group24.de/itsm/internal/snippets"
	"group24.de/itsm/internal/webhooks"
)

// apiServer installs the probe package and serves the API against the test database.
// testAPI is the server with the cookie of a signed-in test account.
type testAPI struct {
	*httptest.Server
	cookie string
	auth   *auth.Service
	authz  *authz.Service
	pool   *pgxpool.Pool
	// rt routes like the binary: one database, or one per tenant (PL47e).
	rt     *db.Router
	kv     *cache.Client
	events *events.Service
	idp    *authntest.IDP
	authn  *authn.Service
	hooks  *webhooks.Service
	// rules and ruleEvents run the rules after a write (WF4).
	rules      *engine.Engine
	ruleEvents *events.Service
	// sla moves the clocks (WF9); tests set its Now and run Due themselves.
	sla *sla.Service
	// settings knows the declared settings (SD20); tests call Forget after installing.
	settings *settings.Service
	// recur creates recurring records (WF14); tests call Run with their time.
	recur *recurrence.Service
	// notify sends mail to smtp (WF15) through a dispatcher of its own, which tests call.
	notify       *notify.Service
	notifyEvents *events.Service
	smtp         *smtptest.Server
	// export writes lists (UI5); tests lower MaxDirect to reach the job.
	export *export.Service
	// person is the person of the test account.
	person uuid.UUID
	// dashboards computes the widgets; tests give it a store of their own (PL68).
	dashboards *dashboard.Service
}

const testPassword = "Api-Test1!"

// bulkTestQueue is the River queue of the asynchronous bulk in the tests (PL32).
const bulkTestQueue = "bulktest"

// ruleTestQueue is the River queue of the rules after a write in the tests (WF4).
const ruleTestQueue = "ruletest"

// notifyTestQueue is the River queue of the notifications in the tests (WF15).
const notifyTestQueue = "notifytest"

// slaTestQueue is the River queue of the deadline jobs in the tests (WF9); no worker takes
// it, the tests run Due themselves.
const slaTestQueue = "slatest"

func apiServer(t *testing.T) *testAPI {
	t.Helper()
	return apiServerWith(t, nil)
}

// apiServerWith is apiServer with a database of their own for the tenants named in
// tenantURLs, by path (PL47e): every service routes as the binary does with
// ITSM_TENANT_MODE=database; without, everything is in ITSM_TEST_DATABASE_URL.
func apiServerWith(t *testing.T, tenantURLs map[string]string) *testAPI {
	t.Helper()
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	addr := os.Getenv("ITSM_TEST_VALKEY_ADDR")
	if url == "" || addr == "" {
		t.Skip("ITSM_TEST_DATABASE_URL or ITSM_TEST_VALKEY_ADDR not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	rt := db.Single(pool)
	if len(tenantURLs) > 0 {
		tenants := map[string]*pgxpool.Pool{}
		for path, u := range tenantURLs {
			if err := db.Migrate(ctx, u, quiet); err != nil {
				t.Fatal(err)
			}
			p, err := db.Open(ctx, u)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(p.Close)
			tenants[path] = p
		}
		rt = db.NewRouter(pool, tenants, func(ctx context.Context) string { return authz.From(ctx).CurrentTenant() })
	}
	for _, dir := range []string{"../../packages/core", "../../packages/probe"} {
		installPackage(t, ctx, rt, dir)
	}
	// Start from an empty probe table: other tests and manual runs leave rows behind.
	if _, err := pool.Exec(ctx, "DELETE FROM rec_probe_probe"); err != nil {
		t.Fatal(err)
	}
	kv, err := cache.Open(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(kv.Close)
	a := auth.New(rt, kv, quiet)
	if _, err := pool.Exec(ctx, "DELETE FROM role_assignment WHERE person_id IN (SELECT person_id FROM account WHERE username LIKE 'apitest%')"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM account WHERE username LIKE 'apitest%'"); err != nil {
		t.Fatal(err)
	}
	acc, err := a.CreateAccount(ctx, "apitest", "API Test", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The tests sign in wrongly on purpose; counters in Valkey from earlier runs (which share
	// it) must not lock the account (PL28; found after many runs in a row, WF2).
	if err := a.Unlock(ctx, "apitest"); err != nil {
		t.Fatal(err)
	}
	// Every test signs in from this address, the tests of package auth with the lockout on.
	if err := a.UnlockAddress(ctx, netip.MustParseAddr("127.0.0.1")); err != nil {
		t.Fatal(err)
	}
	// The test account is an administrator: a person, linked, with the role admin at root.
	az := authz.New(rt, kv, quiet)
	person, err := records.New(rt).Create(ctx, "core.person", map[string]any{"last_name": "API Test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pid := uuid.MustParse(person["id"].(string))
	if err := az.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	if _, err := az.Assign(ctx, "admin", &pid, nil, "root", nil); err != nil {
		t.Fatal(err)
	}
	runCtx, stopRun := context.WithCancel(context.Background())
	t.Cleanup(stopRun)
	go func() { _ = az.Run(runCtx) }()
	recs := withHook(rt, az)
	go recs.WatchTypes(runCtx, quiet)
	ev := events.New(rt, recs, kv, quiet)
	hook := recs.AfterWrite
	recs.AfterWrite = func(ctx context.Context, typeName string) {
		hook(ctx, typeName)
		ev.Wake(ctx)
	}
	orgSvc := org.New(rt)
	// The tests sign in wrongly on purpose, all from 127.0.0.1: without a setting of their own
	// the lockout stays off, so that one test does not lock the next (PL28).
	a.Settings = func(ctx context.Context, path, key string) (json.RawMessage, string, error) {
		raw, src, err := orgSvc.Setting(ctx, path, key)
		if err == nil && raw == nil && (key == auth.SettingAccountFailures || key == auth.SettingIPFailures) {
			return json.RawMessage("0"), "test", nil
		}
		return raw, src, err
	}
	a.AllowLocal = func(ctx context.Context, account uuid.UUID) (bool, error) {
		acc, err := az.Load(ctx, account)
		if err != nil {
			return false, err
		}
		return acc.Allowed("core.role", "manage_rights"), nil
	}
	idp := authntest.New(t)
	an := authn.New(rt, kv, a, az, recs, orgSvc, quiet, authn.Options{Issuer: idp.Issuer, ClientID: idp.ClientID, ClientSecret: idp.Secret, Name: "Fake IdP"})
	js := testJobs(t, ctx, rt)
	// The asynchronous bulk (PL32) with a worker on a queue of its own, so that a running
	// instance on the same database does not take the test's jobs.
	if _, err := pool.Exec(ctx, "DELETE FROM river_job WHERE queue = $1", bulkTestQueue); err != nil {
		t.Fatal(err)
	}
	bulkStore, err := files.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bulkJobs := jobs.New(rt, jobs.Options{Logger: quiet, Workers: map[string]int{bulkTestQueue: 2}})
	stream := bulk.NewAsync(bulkStore, bulkJobs, recs, az.Load, orgSvc.Setting, quiet)
	stream.Queue = bulkTestQueue
	stream.Register()
	// Exports (UI5): large ones as jobs on the same queue.
	// The settings the packages declare (SD20), as the binary wires them.
	settingsSvc := settings.New(rt, orgSvc)
	recs.SettingValue = settingsSvc.Value
	fileSvc := testFiles(t, rt, recs, orgSvc.Setting)
	exp := export.New(recs, fileSvc, bulkJobs, az.Load, quiet)
	exp.Queue = bulkTestQueue
	exp.Register()
	if err := bulkJobs.Start(runCtx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bulkJobs.Stop(context.Background()) })
	// Rules (WF4) on a queue of their own, with a dispatcher of their own that starts at the end
	// of the stream: tests call Dispatch and wait for the effect of a rule after a write.
	if _, err := pool.Exec(ctx, "DELETE FROM river_job WHERE queue = $1", ruleTestQueue); err != nil {
		t.Fatal(err)
	}
	ruleJobs := jobs.New(rt, jobs.Options{Logger: quiet, Workers: map[string]int{ruleTestQueue: 2}})
	ruleEngine := engine.New(rt, recs, ruleJobs, nil, quiet)
	ruleEngine.Rights = az.Load
	ruleEngine.Queue = ruleTestQueue
	ruleEngine.RegisterWorkers(ruleJobs)
	if err := ruleJobs.Start(runCtx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ruleJobs.Stop(context.Background()) })
	recs.Before = ruleEngine.Before
	ruleEvents := events.New(rt, recs, nil, quiet)
	ruleEvents.Prefix = "apitest-"
	ruleEvents.Consume(events.ConsumerRules, ruleEngine.HandleBatch)
	// Each database has its own stream and positions (PL47c).
	for _, d := range rt.Databases() {
		if err := ruleEvents.SkipToEnd(d.Bind(ctx)); err != nil {
			t.Fatal(err)
		}
	}
	// SLA clocks (WF9) in the transaction of the write; the deadline jobs on a queue of their own.
	if _, err := pool.Exec(ctx, "DELETE FROM river_job WHERE queue = $1", slaTestQueue); err != nil {
		t.Fatal(err)
	}
	calendars := calendar.New(recs, orgSvc.Setting)
	slaSvc := sla.New(rt, recs, calendars, js, nil, quiet)
	slaSvc.Queue = slaTestQueue
	slaSvc.Settings, slaSvc.Declared = settingsSvc.Value, settingsSvc.Definition
	recur := recurrence.New(recs, quiet)
	// Notifications (WF15) to a test SMTP server, on a queue of their own with workers, through
	// a dispatcher of their own that starts at the end of the stream.
	if _, err := pool.Exec(ctx, "DELETE FROM river_job WHERE queue = $1", notifyTestQueue); err != nil {
		t.Fatal(err)
	}
	mailbox := smtptest.Start(t)
	notifyJobs := jobs.New(rt, jobs.Options{Logger: quiet, Workers: map[string]int{notifyTestQueue: 2}})
	notifySvc := notify.New(rt, recs, notifyJobs, az.Load,
		notify.NewMailer(notify.SMTP{Host: mailbox.Host, Port: mailbox.Port, From: "ITSM Test <itsm@test.local>"}), quiet)
	notifySvc.Queue, notifySvc.PublicURL, notifySvc.Settings = notifyTestQueue, "http://itsm.test", orgSvc.Setting
	notifySvc.SetKV(kv)
	notifySvc.Register(notifyJobs)
	if err := notifyJobs.Start(runCtx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = notifyJobs.Stop(context.Background()) })
	notifyEvents := events.New(rt, recs, nil, quiet)
	notifyEvents.Prefix = "apitest-"
	notifyEvents.Consume(events.ConsumerNotify, notifySvc.HandleBatch)
	for _, d := range rt.Databases() {
		if err := notifyEvents.SkipToEnd(d.Bind(ctx)); err != nil {
			t.Fatal(err)
		}
	}
	dashboards := dashboard.New(recs)
	// The service time of a resolution (UI14a), as cmd/itsm wires it.
	recs.WorkingTime = func(ctx context.Context, path string, from, to time.Time) (time.Duration, error) {
		c, _, err := calendars.ForNode(authz.WithAccess(ctx, authz.System()), path)
		if err != nil || c == nil {
			return to.Sub(from), err
		}
		return c.Between(from, to)
	}
	recs.Clocks, recs.CheckRecord = slaSvc, records.Checks(slaSvc.Check, recur.Check, records.CheckApprovalTemplate, notifySvc.Check, savedsearch.Check(recs), forms.Check(recs), portal.Check(recs), snippets.Check(recs), dashboards.Check)
	snippetSvc := snippets.New(recs)
	snippetSvc.PublicURL = "http://itsm.test"
	slaSvc.Levels = func(ctx context.Context, typeName string, id uuid.UUID) ([]sla.Level, error) {
		list, err := ruleEngine.Levels(ctx, typeName, id)
		out := make([]sla.Level, len(list))
		for i, l := range list {
			out[i] = sla.Level{Rule: l.Rule, Percent: l.Percent, Minutes: l.Minutes}
		}
		return out, err
	}
	wrote := recs.AfterWrite
	recs.AfterWrite = func(ctx context.Context, typeName string) {
		wrote(ctx, typeName)
		slaSvc.Wrote(ctx, typeName)
	}
	hooks := webhooks.New(rt, recs, js, orgSvc.Setting, az.Load, quiet)
	rels := relations.New(rt, recs)
	h := NewRouter(Options{Logger: quiet, Version: "test", API: true, Records: recs, Auth: a, Org: orgSvc, Settings: settingsSvc, Dashboards: dashboards, Groups: query.NewGroups(rt), Authz: az, Jobs: js, Events: ev, Authn: an, Relations: rels, Files: fileSvc, Export: exp, Presence: presence.New(kv), Prefs: prefs.New(rt, recs, orgSvc.Setting), Forms: forms.New(recs).WithRelations(rels), Portal: portal.New(recs), Snippets: snippetSvc, Customer: customer.New(rt, recs, quiet), Webhooks: hooks, Search: search.NewPostgres(recs), SecLog: seclog.New(rt, orgSvc.Setting, quiet), KV: kv, Bulk: stream, Rules: ruleEngine,
		Calendars: calendars, SLA: slaSvc, Notify: notifySvc,
		Branding: branding.New(rt, orgSvc.Setting)})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	an.SetPublicURL(srv.URL)
	api := &testAPI{Server: srv, auth: a, authz: az, pool: pool, rt: rt, kv: kv, events: ev, idp: idp, authn: an, hooks: hooks, export: exp,
		rules: ruleEngine, ruleEvents: ruleEvents, sla: slaSvc, settings: settingsSvc, recur: recur, notify: notifySvc, notifyEvents: notifyEvents,
		smtp: mailbox, person: pid, dashboards: dashboards}
	r := call(t, api, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest", "password": "`+testPassword+`"}`, nil)
	if r.code != 200 {
		t.Fatalf("login: %d %s", r.code, r.raw)
	}
	api.cookie = sessionCookieOf(t, r.hdr)
	return api
}

// installPackage loads a package directory and installs it: definitions, tables, list entries.
func installPackage(t *testing.T, ctx context.Context, rt *db.Router, dir string) meta.Package {
	t.Helper()
	p, err := meta.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.ResolveInstalled(ctx, queries.New(rt.Control()), &p); err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, rt.Control(), p); err != nil {
		t.Fatal(err)
	}
	// The tables go into every database, as itsm package install does (PL47d).
	for _, pool := range rt.Pools() {
		if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := records.New(rt).SeedLists(ctx, p); err != nil {
		t.Fatal(err)
	}
	return p
}

// testJobs is an insert-only job service with one periodic job for the API tests (PL23).
func testJobs(t *testing.T, ctx context.Context, rt *db.Router) *jobs.Service {
	t.Helper()
	for _, sql := range []string{"DELETE FROM river_job WHERE kind LIKE 'apitest.%'", "DELETE FROM job_schedule WHERE name LIKE 'apitest.%'"} {
		if _, err := rt.Control().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	js := jobs.New(rt, jobs.Options{Logger: quiet})
	if err := js.AddPeriodic(jobs.Periodic{Name: "apitest.tick", Schedule: "*/10 * * * *", Queue: "apitest",
		Args: func() river.JobArgs { return tickArgs{} }}); err != nil {
		t.Fatal(err)
	}
	if err := js.Start(ctx); err != nil {
		t.Fatal(err)
	}
	return js
}

type tickArgs struct{}

func (tickArgs) Kind() string { return "apitest.tick" }

// withHook wires the record service like the binary: memberships change rights (PL9).
func withHook(rt *db.Router, az *authz.Service) *records.Service {
	rec := records.New(rt)
	rec.AfterWrite = func(ctx context.Context, typeName string) {
		if typeName == "core.group_member" {
			az.Invalidate(ctx)
		}
	}
	return rec
}

// sessionCookieOf extracts the session cookie from Set-Cookie.
func sessionCookieOf(t *testing.T, hdr http.Header) string {
	t.Helper()
	for _, c := range (&http.Response{Header: hdr}).Cookies() {
		if c.Name == sessionCookie {
			return c.Value
		}
	}
	t.Fatalf("no %s cookie in %v", sessionCookie, hdr.Values("Set-Cookie"))
	return ""
}

type resp struct {
	code int
	hdr  http.Header
	body map[string]any
	raw  string
}

// dropRole deletes a role with its assignments, for the cleanup of a test: a role in use is not
// deleted any more (PL47b, Hauptplan E141), the cascade of before is the test's job now.
func dropRole(t *testing.T, api *testAPI, name string) {
	t.Helper()
	r := call(t, api, http.MethodGet, "/api/v1/assignments", "", nil)
	var list []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &list)
	for _, a := range list {
		if a["role"] == name {
			call(t, api, http.MethodDelete, "/api/v1/assignments/"+a["id"].(string), "", nil)
		}
	}
	call(t, api, http.MethodDelete, "/api/v1/roles/"+name, "", nil)
}

// call sends a request with the session cookie and the CSRF header (E74). A header "Cookie"
// set to "" in hdr sends the request without the session.
func call(t *testing.T, api *testAPI, method, path, body string, hdr map[string]string) resp {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, api.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(csrfHeader, csrfValue)
	if api.cookie != "" {
		req.Header.Set("Cookie", sessionCookie+"="+api.cookie)
	}
	for k, v := range hdr {
		if v == "" {
			req.Header.Del(k)
			continue
		}
		req.Header.Set(k, v)
	}
	res, err := api.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var buf strings.Builder
	dec := json.NewDecoder(io2{&buf, res})
	var m map[string]any
	_ = dec.Decode(&m)
	return resp{code: res.StatusCode, hdr: res.Header, body: m, raw: buf.String()}
}

// io2 tees the body into a builder while decoding.
type io2 struct {
	b   *strings.Builder
	res *http.Response
}

func (t io2) Read(p []byte) (int, error) {
	n, err := t.res.Body.Read(p)
	t.b.Write(p[:n])
	return n, err
}

func TestRecordLifecycleOverHTTP(t *testing.T) {
	api := apiServer(t)
	const base = "/api/v1/records/probe.probe"

	// Validation problem as RFC 9457 with errors.
	r := call(t, api, http.MethodPost, base, `{"priority": 9, "colour": "red"}`, nil)
	if r.code != 422 || !strings.HasPrefix(r.hdr.Get("Content-Type"), "application/problem+json") || r.body["type"] != "/problems/validation" {
		t.Fatalf("validation: %d %s %s", r.code, r.hdr.Get("Content-Type"), r.raw)
	}
	errs, _ := r.body["errors"].([]any)
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors (title required, priority max, colour unknown): %s", r.raw)
	}
	if r.body["title"] != "Eingabe ungültig" {
		t.Fatalf("German title by default: %s", r.raw)
	}
	r = call(t, api, http.MethodPost, base, `{}`, map[string]string{"Accept-Language": "en-US,en;q=0.9"})
	if r.body["title"] != "Validation failed" {
		t.Fatalf("English title on request: %s", r.raw)
	}

	// Create.
	r = call(t, api, http.MethodPost, base, `{"title": "first", "kind": "fault"}`, nil)
	if r.code != 201 || r.hdr.Get("ETag") != `"1"` || !strings.HasPrefix(r.hdr.Get("Location"), base+"/") || r.body["priority"] != 3.0 {
		t.Fatalf("create: %d %v %s", r.code, r.hdr, r.raw)
	}
	id := r.body["id"].(string)

	// Get with ETag.
	r = call(t, api, http.MethodGet, base+"/"+id, "", nil)
	if r.code != 200 || r.hdr.Get("ETag") != `"1"` || r.body["title"] != "first" {
		t.Fatalf("get: %d %s", r.code, r.raw)
	}

	// Update: without If-Match 428, with a stale one 409, with the right one 200 and ETag 2.
	r = call(t, api, http.MethodPatch, base+"/"+id, `{"title": "second"}`, nil)
	if r.code != 428 || r.body["type"] != "/problems/precondition_required" {
		t.Fatalf("patch without If-Match: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodPatch, base+"/"+id, `{"title": "second"}`, map[string]string{"If-Match": `"7"`})
	if r.code != 409 {
		t.Fatalf("patch with stale If-Match: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodPatch, base+"/"+id, `{"title": "second"}`, map[string]string{"If-Match": `"1"`})
	if r.code != 200 || r.hdr.Get("ETag") != `"2"` || r.body["title"] != "second" {
		t.Fatalf("patch: %d %s", r.code, r.raw)
	}

	// History.
	r = call(t, api, http.MethodGet, base+"/"+id+"/history", "", nil)
	if r.code != 200 || !strings.Contains(r.raw, `"action":"create"`) || !strings.Contains(r.raw, `"old":"first"`) {
		t.Fatalf("history: %d %s", r.code, r.raw)
	}

	// Trash, 404 afterwards, restore, purge rules.
	if r = call(t, api, http.MethodDelete, base+"/"+id, "", nil); r.code != 204 {
		t.Fatalf("trash: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodGet, base+"/"+id, "", nil); r.code != 404 || r.body["type"] != "/problems/not_found" {
		t.Fatalf("get after trash: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, base+"/"+id+":purge", "", nil); r.code != 204 {
		t.Fatalf("purge: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, base+"/"+id+":restore", "", nil); r.code != 404 {
		t.Fatalf("restore after purge: %d %s", r.code, r.raw)
	}

	// Bad inputs.
	if r = call(t, api, http.MethodGet, base+"/not-a-uuid", "", nil); r.code != 400 || r.body["type"] != "/problems/bad_request" {
		t.Fatalf("bad id: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodGet, "/api/v1/records/probe.nope", "", nil); r.code != 404 {
		t.Fatalf("unknown type: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, base, `[1,2]`, nil); r.code != 400 {
		t.Fatalf("non-object body: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodGet, "/api/v1/nothing", "", nil); r.code != 404 || r.body["type"] != "/problems/not_found" {
		t.Fatalf("unknown endpoint: %d %s", r.code, r.raw)
	}
}

func TestListFilterSortPageOverHTTP(t *testing.T) {
	api := apiServer(t)
	const base = "/api/v1/records/probe.probe"
	for _, title := range []string{"a", "b", "c"} {
		if r := call(t, api, http.MethodPost, base, `{"title": "`+title+`", "priority": 2}`, nil); r.code != 201 {
			t.Fatalf("create %s: %d %s", title, r.code, r.raw)
		}
	}
	r := call(t, api, http.MethodGet, base+"?limit=2&sort=-created_at&fields=title", "", nil)
	items, _ := r.body["items"].([]any)
	if r.code != 200 || len(items) != 2 || items[0].(map[string]any)["title"] != "c" || r.body["next"] == nil {
		t.Fatalf("page 1: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodGet, base+"?limit=2&sort=-created_at&fields=title&after="+r.body["next"].(string), "", nil)
	items, _ = r.body["items"].([]any)
	if r.code != 200 || len(items) != 1 || items[0].(map[string]any)["title"] != "a" || r.body["next"] != nil {
		t.Fatalf("page 2: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodGet, base+"?filter[title]=b&filter[priority]=2", "", nil)
	items, _ = r.body["items"].([]any)
	if r.code != 200 || len(items) != 1 {
		t.Fatalf("filter: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodGet, base+"?sort=title", "", nil); r.code != 422 {
		t.Fatalf("bad sort: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodGet, base+"?limit=abc", "", nil); r.code != 400 {
		t.Fatalf("bad limit: %d %s", r.code, r.raw)
	}
}

func TestTypesAndOpenAPI(t *testing.T) {
	api := apiServer(t)
	r := call(t, api, http.MethodGet, "/api/v1/types/probe.probe", "", nil)
	if r.code != 200 || r.body["name"] != "probe" || r.body["package"] != "probe" {
		t.Fatalf("type: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodGet, "/api/v1/openapi.json", "", nil)
	if r.code != 200 {
		t.Fatalf("openapi: %d", r.code)
	}
	paths, _ := r.body["paths"].(map[string]any)
	for _, p := range []string{"/api/v1/records/probe.probe", "/api/v1/records/probe.probe/{id}", "/api/v1/records/{typeName}", "/api/v1/types"} {
		if paths[p] == nil {
			t.Errorf("path %s missing", p)
		}
	}
	schemas := r.body["components"].(map[string]any)["schemas"].(map[string]any)
	probe, _ := schemas["probe.probe"].(map[string]any)
	props, _ := probe["properties"].(map[string]any)
	if props["title"] == nil || props["kind"] == nil || props["id"] == nil {
		t.Fatalf("probe schema incomplete: %v", probe)
	}
	kind := props["kind"].(map[string]any)
	if enum, _ := kind["enum"].([]any); len(enum) != 3 {
		t.Fatalf("kind enum: %v", kind)
	}
}
