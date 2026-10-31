// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/rules/engine"
	"group24.de/itsm/internal/webhooks"
)

const whereQueue = "pl47c_where"

// whereArgs is a job that reports the database it runs in.
type whereArgs struct{}

func (whereArgs) Kind() string { return "pl47c.where" }

type whereWorker struct {
	river.WorkerDefaults[whereArgs]
	rt  *db.Router
	got chan string
}

func (w *whereWorker) Work(ctx context.Context, _ *river.Job[whereArgs]) error {
	var name string
	if err := w.rt.Tenant(ctx).QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
		return err
	}
	w.got <- name
	return nil
}

// The acceptance of PL47c: with a database per tenant every database has its River client and
// its dispatcher in the process. A job queued bound to B lies and runs in B; one queued from a
// request in A lies and runs in A, and the job list of A shows it, that of the root does not.
// An event in A reaches the webhook of A through the dispatcher of A: the delivery job lies in
// A, none in the control database or in B. A webhook of the installation gets the events of
// both tenants (PL47d). A rule of the installation on a schedule runs once from the control
// database and acts in both tenants.
func TestBackgroundPerDatabase(t *testing.T) {
	s := newControlSetup(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	name := func(p *pgxpool.Pool) string {
		var n string
		if err := p.QueryRow(ctx, "SELECT current_database()").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	bound := map[string]context.Context{}
	for _, d := range s.rt.Databases() {
		bound[d.Name] = d.Bind(ctx)
	}
	if len(bound) != 3 {
		t.Fatalf("databases: %d", len(bound))
	}
	inA := authz.WithAccess(ctx, authz.SystemIn("root.ta"))
	atRoot := authz.WithAccess(ctx, authz.SystemIn("root"))

	got := make(chan string, 4)
	js := jobs.New(s.rt, jobs.Options{Logger: quiet, Workers: map[string]int{whereQueue: 2}})
	jobs.AddWorker(js, &whereWorker{rt: s.rt, got: got})
	recs := records.New(s.rt)
	rootOwner := uuid.New() // owns the webhook of the installation (PL47d)
	hooks := webhooks.New(s.rt, recs, js, nil, func(_ context.Context, owner uuid.UUID) (*authz.Access, error) {
		if owner == rootOwner {
			return authz.System(), nil
		}
		return authz.SystemIn("root.ta"), nil
	}, quiet)
	hooks.Queue = "pl47c_hooks" // no one delivers: the test looks at the queued jobs only
	webhooks.RegisterDeliveryJob(js, hooks)
	if err := js.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = js.Stop(context.Background()) })

	// Jobs run in the database they were queued in.
	for _, c := range []struct {
		label string
		ctx   context.Context
		pool  *pgxpool.Pool
	}{{"bound to B", bound["root.tb"], s.b}, {"request in A", inA, s.a}} {
		j, err := js.Enqueue(c.ctx, whereArgs{}, &river.InsertOpts{Queue: whereQueue})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case db := <-got:
			if want := name(c.pool); db != want {
				t.Fatalf("%s: the job ran in %s, want %s", c.label, db, want)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("%s: the job did not run", c.label)
		}
		if count(t, c.pool, "SELECT count(*) FROM river_job WHERE id = $1 AND kind = $2", j.ID, whereArgs{}.Kind()) != 1 {
			t.Fatalf("%s: the job is not in its database", c.label)
		}
	}
	if count(t, s.control, "SELECT count(*) FROM river_job WHERE kind = $1", whereArgs{}.Kind()) != 0 {
		t.Fatal("a job of a tenant lies in the control database")
	}
	// The job list is that of the database the request works in (the probe carries no tenant,
	// so it counts among the jobs of the system: installation, PL45).
	if l, err := js.List(inA, jobs.Filter{Kind: whereArgs{}.Kind(), Installation: true}); err != nil || len(l) != 1 {
		t.Fatalf("jobs of A seen from A: %d %v", len(l), err)
	}
	if l, err := js.List(atRoot, jobs.Filter{Kind: whereArgs{}.Kind(), Installation: true}); err != nil || len(l) != 0 {
		t.Fatalf("jobs of the tenants seen from the root: %d %v", len(l), err)
	}

	// An event in A reaches the webhook of A through the dispatcher of A.
	owner := uuid.New()
	if _, err := s.a.Exec(ctx, "INSERT INTO account (id, username, display_name, password_hash) VALUES ($1, 'pl47c_owner', 'o', 'x')", owner); err != nil {
		t.Fatal(err)
	}
	hookName, hookURL := "pl47c", "https://receiver.invalid/hook"
	w, err := hooks.Create(inA, webhooks.Input{Name: &hookName, URL: &hookURL}, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := events.New(s.rt, recs, nil, quiet)
	ev.Prefix, ev.Poll = "pl47c_", 50*time.Millisecond
	ev.Consume(events.ConsumerWebhooks, hooks.HandleBatch)
	go func() { _ = ev.RunDispatcher(ctx) }()
	waitUntil(t, "a dispatcher in every database", func() bool {
		for _, p := range []*pgxpool.Pool{s.control, s.a, s.b} {
			if count(t, p, "SELECT count(*) FROM event_consumer WHERE name = 'pl47c_webhooks'") != 1 {
				return false
			}
		}
		return true
	})
	if _, err := recs.Create(inA, "probe.probe", map[string]any{"title": "pl47c"}, nil); err != nil {
		t.Fatal(err)
	}
	deliveries := "SELECT count(*) FROM river_job WHERE kind = 'webhooks.deliver' AND args->>'webhook' = $1"
	waitUntil(t, "the delivery job in A", func() bool { return count(t, s.a, deliveries, w.ID.String()) == 1 })
	if count(t, s.control, deliveries, w.ID.String()) != 0 || count(t, s.b, deliveries, w.ID.String()) != 0 {
		t.Fatal("a delivery of A lies outside A")
	}
	if l, err := hooks.Deliveries(inA, w.ID, 10); err != nil || len(l) != 1 {
		t.Fatalf("deliveries of the webhook of A: %d %v", len(l), err)
	}

	// A webhook of the installation lies in the control database and gets the events of every
	// tenant, through the dispatcher of each, from its creation on (PL47d).
	if _, err := s.control.Exec(ctx, "INSERT INTO account (id, username, display_name, password_hash) VALUES ($1, 'pl47d_owner', 'o', 'x')", rootOwner); err != nil {
		t.Fatal(err)
	}
	rootName := "pl47d"
	rw, err := hooks.Create(atRoot, webhooks.Input{Name: &rootName, URL: &hookURL}, rootOwner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count(t, s.control, "SELECT count(*) FROM webhook WHERE id = $1", rw.ID) != 1 {
		t.Fatal("the webhook of the installation is not central")
	}
	inB := authz.WithAccess(ctx, authz.SystemIn("root.tb"))
	for _, in := range []context.Context{inA, inB} {
		if _, err := recs.Create(in, "probe.probe", map[string]any{"title": "pl47d"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []*pgxpool.Pool{s.a, s.b} {
		waitUntil(t, "the delivery of the installation's webhook in a tenant", func() bool { return count(t, p, deliveries, rw.ID.String()) == 1 })
		if count(t, p, "SELECT count(*) FROM webhook_position WHERE webhook_id = $1", rw.ID) != 1 {
			t.Fatal("the webhook of the installation has no position in the tenant's database")
		}
	}
	if l, err := hooks.Deliveries(atRoot, rw.ID, 10); err != nil || len(l) != 2 {
		t.Fatalf("deliveries of the installation's webhook, merged: %d %v", len(l), err)
	}
	if err := hooks.Delete(atRoot, rw.ID); err != nil {
		t.Fatal(err)
	}
	if count(t, s.a, "SELECT count(*) FROM webhook_position WHERE webhook_id = $1", rw.ID) != 0 {
		t.Fatal("deleting the webhook leaves its positions")
	}

	// A rule of the installation on a schedule runs from the control database, in both tenants.
	rules := engine.New(s.rt, recs, js, nil, quiet)
	for _, in := range []context.Context{inA, inB} {
		if _, err := recs.Create(in, "probe.probe", map[string]any{"title": "pl47c time"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := rules.Put(atRoot, "pl47c_time", engine.Rule{TypeName: "probe.probe", Trigger: engine.TriggerTime, Mode: engine.ModeAfter,
		Schedule: "* * * * *", Condition: `record.title == "pl47c time"`, Actions: []engine.Action{{Set: map[string]any{"priority": 5}}}, Enabled: true},
		true, nil); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Minute)
	for _, d := range []string{"root.ta", "root.tb"} {
		if ran, err := rules.RunSchedules(bound[d], later); err != nil || len(ran) != 0 {
			t.Fatalf("the job in %s ran a rule of the installation: %v %v", d, ran, err)
		}
	}
	if ran, err := rules.RunSchedules(bound[""], later); err != nil || ran["pl47c_time"] != 2 {
		t.Fatalf("the rule of the installation found %v (%v), want one record in each tenant", ran, err)
	}
	for _, p := range []*pgxpool.Pool{s.a, s.b} {
		if count(t, p, "SELECT count(*) FROM rec_probe_probe WHERE title = 'pl47c time' AND priority = 5") != 1 {
			t.Fatal("the scheduled rule did not act in a tenant")
		}
	}
	if ran, err := rules.RunSchedules(bound[""], later); err != nil || len(ran) != 0 {
		t.Fatalf("the rule ran twice in one minute: %v %v", ran, err)
	}
}

// waitUntil polls a condition for up to a minute: the horizon of the events is that of the
// whole server, and a bulk batch of another package's tests holds it back (E87).
func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
