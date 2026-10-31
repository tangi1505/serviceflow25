// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/schema"
)

const thing = "webhooktest.thing"

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type fixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	rec     *records.Service
	js      *jobs.Service
	s       *Service
	owner   uuid.UUID
	blinded uuid.UUID // an owner without any access
}

func setup(t *testing.T, workers map[string]int) *fixture {
	t.Helper()
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, sql := range []string{
		"DROP TABLE IF EXISTS rec_webhooktest_thing",
		"DELETE FROM package WHERE name = 'webhooktest'",
		"DELETE FROM schema_change WHERE type_name LIKE 'webhooktest.%'",
		"DELETE FROM account WHERE username LIKE 'webhooktest_%'", // cascades to webhook
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte("name: webhooktest\nversion: 0.1.0\nlabel: {de: W, en: W}\n")},
		"types/thing.yaml": {Data: []byte("name: thing\nlabel: {de: D, en: T}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
		t.Fatal(err)
	}
	f := &fixture{ctx: ctx, pool: pool, rec: records.New(db.Single(pool)), owner: uuid.New(), blinded: uuid.New()}
	for name, id := range map[string]uuid.UUID{"webhooktest_owner": f.owner, "webhooktest_blind": f.blinded} {
		if _, err := pool.Exec(ctx, "INSERT INTO account (id, username, display_name, password_hash) VALUES ($1, $2, $2, 'x')", id, name); err != nil {
			t.Fatal(err)
		}
	}
	f.js = jobs.New(db.Single(pool), jobs.Options{Logger: quiet, Workers: workers})
	settings := func(_ context.Context, _, key string) (json.RawMessage, string, error) {
		if key == SettingAllowedHTTPHosts {
			return json.RawMessage(`["127.0.0.1"]`), "root", nil
		}
		return nil, "", nil
	}
	rights := func(_ context.Context, account uuid.UUID) (*authz.Access, error) {
		if account == f.owner {
			return authz.System(), nil
		}
		return nil, errors.New("no access")
	}
	f.s = New(db.Single(pool), f.rec, f.js, settings, rights, quiet)
	RegisterDeliveryJob(f.js, f.s)
	if err := f.js.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.js.Stop(context.Background()) })
	return f
}

// receiver answers 500 to the first fail requests, then 204, and keeps what it got.
type receiver struct {
	mu    sync.Mutex
	fail  int
	calls []call
}

type call struct {
	header http.Header
	body   []byte
}

func (rc *receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.calls = append(rc.calls, call{r.Header.Clone(), body})
	if len(rc.calls) <= rc.fail {
		http.Error(w, "not now", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rc *receiver) got() []call {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]call{}, rc.calls...)
}

// event creates a record and returns its create event.
func (f *fixture) event(t *testing.T, title string) records.Event {
	t.Helper()
	before, err := f.s.tq(f.ctx).LatestEventPosition(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := f.rec.Create(f.ctx, thing, map[string]any{"title": title}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A bulk batch of another package's tests holds the horizon back (E87), on a busy machine
	// with -race for longer than ten seconds (PL47d): wait a minute at most.
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		list, err := f.rec.Events(f.ctx, before, 1000)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range list {
			if e.RecordID.String() == rec["id"] {
				return e
			}
		}
	}
	t.Fatal("no event for the record")
	return records.Event{}
}

// The acceptance of PL25: a receiver that answers 500 twice gets the event on the third
// attempt, through River and its retries; the signature checks with the secret.
func TestDeliveryRetriesUntilAccepted(t *testing.T) {
	// An own queue: an instance running on the same database must not take these jobs.
	f := setup(t, map[string]int{"webhooktest": 2})
	f.s.Queue = "webhooktest"
	f.s.Backoff = func(int) time.Duration { return 10 * time.Millisecond }
	rc := &receiver{fail: 2}
	srv := httptest.NewServer(rc)
	defer srv.Close()

	name, target := "receiver", srv.URL+"/hook"
	w, err := f.s.Create(f.ctx, Input{Name: &name, URL: &target, Types: &[]string{thing}}, f.owner, nil)
	if err != nil || !strings.HasPrefix(w.Secret, "whsec_") {
		t.Fatalf("create: %+v %v", w, err)
	}
	e := f.event(t, "printer broken")
	if err := f.s.Handle(f.ctx, e); err != nil {
		t.Fatal(err)
	}
	// The dispatcher may hand an event on twice: it is queued once.
	if err := f.s.Handle(f.ctx, e); err != nil {
		t.Fatal(err)
	}

	var deliveries []Delivery
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		deliveries, err = f.s.Deliveries(f.ctx, w.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(deliveries) == 1 && deliveries[0].State == "completed" {
			break
		}
	}
	if len(deliveries) != 1 || deliveries[0].State != "completed" || deliveries[0].Attempt != 3 || deliveries[0].Position != e.Position ||
		deliveries[0].MaxAttempts != MaxAttempts {
		t.Fatalf("deliveries: %+v", deliveries)
	}
	calls := rc.got()
	if len(calls) != 3 {
		t.Fatalf("receiver got %d calls", len(calls))
	}
	last := calls[2]
	if err := Verify(w.Secret, last.header, last.body, 5*time.Minute); err != nil {
		t.Fatalf("signature: %v", err)
	}
	other, _ := NewSecret()
	if err := Verify(other, last.header, last.body, 5*time.Minute); err == nil {
		t.Fatal("another secret must not verify")
	}
	if calls[0].header.Get("webhook-id") != last.header.Get("webhook-id") || last.header.Get("webhook-id") != "evt_"+jsonInt(e.Position) {
		t.Fatalf("webhook-id must stay the same over the attempts: %q %q", calls[0].header.Get("webhook-id"), last.header.Get("webhook-id"))
	}
	var got records.Event
	if err := json.Unmarshal(last.body, &got); err != nil || got.Position != e.Position || got.Kind != "create" || got.RecordID != e.RecordID {
		t.Fatalf("payload: %s %v", last.body, err)
	}
	if rotated, err := f.s.Rotate(f.ctx, w.ID, nil); err != nil || rotated.Secret == w.Secret {
		t.Fatalf("rotate: %v", err)
	}
}

// Filters and rights: an event of another kind or one the owner may not read is not queued;
// a deleted subscription ends its deliveries without sending.
func TestFanOutFiltersAndRights(t *testing.T) {
	f := setup(t, nil)
	rc := &receiver{}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	target := srv.URL
	names := []string{"updates only", "blind", "all"}
	kinds := [][]string{{"update"}, {}, {}}
	owners := []uuid.UUID{f.owner, f.blinded, f.owner}
	var hooks []Webhook
	for i := range names {
		w, err := f.s.Create(f.ctx, Input{Name: &names[i], URL: &target, Kinds: &kinds[i], Types: &[]string{thing}}, owners[i], nil)
		if err != nil {
			t.Fatal(err)
		}
		hooks = append(hooks, w)
	}
	e := f.event(t, "x")
	if err := f.s.Handle(f.ctx, e); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for i, w := range hooks {
		d, err := f.s.Deliveries(f.ctx, w.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		counts[names[i]] = len(d)
		got, _ := f.s.Get(f.ctx, w.ID)
		// The dispatcher works in batches (PL41): a subscription has taken the batch whether an
		// event matched it or not; it only gets no delivery. At least this event: a running
		// instance on the same database (the local container) hands on later events too and
		// moves the subscriptions further (seen in the CI run of UI12, 2026-10-29).
		if names[i] == "updates only" && got.Position < e.Position {
			t.Errorf("a subscription the event does not match moves with the batch: %d < %d", got.Position, e.Position)
		}
		if names[i] == "blind" && got.Position < e.Position {
			t.Errorf("an invisible event still moves the position: %d < %d", got.Position, e.Position)
		}
	}
	if counts["updates only"] != 0 || counts["blind"] != 0 || counts["all"] != 1 {
		t.Fatalf("queued deliveries: %v", counts)
	}
	if err := f.s.Delete(f.ctx, hooks[2].ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Deliver(f.ctx, DeliverArgs{Webhook: hooks[2].ID, Position: e.Position}); err != nil || len(rc.got()) != 0 {
		t.Fatalf("delivery of a deleted subscription: %v, %d calls", err, len(rc.got()))
	}
	if _, err := f.s.Get(f.ctx, hooks[2].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted: %v", err)
	}
}

// Targets: https always, http only for the allowed hosts; names and types are checked.
func TestSubscriptionInput(t *testing.T) {
	f := setup(t, nil)
	name := "x"
	for raw, ok := range map[string]bool{
		"https://receiver.example/hook":    true,
		"http://127.0.0.1:9/hook":          true,
		"http://receiver.example/hook":     false,
		"ftp://receiver.example/hook":      false,
		"https://user:pw@receiver.example": false,
		"/relative":                        false,
	} {
		u := raw
		_, err := f.s.Create(f.ctx, Input{Name: &name, URL: &u}, f.owner, nil)
		if (err == nil) != ok || (!ok && !errors.Is(err, ErrInvalid)) {
			t.Errorf("%s: %v", raw, err)
		}
	}
	u := "https://receiver.example/hook"
	if _, err := f.s.Create(f.ctx, Input{Name: &name, URL: &u, Types: &[]string{"no.such"}}, f.owner, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown type: %v", err)
	}
	if _, err := f.s.Create(f.ctx, Input{URL: &u}, f.owner, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("without name: %v", err)
	}
	var total time.Duration
	for attempt := 1; attempt < MaxAttempts; attempt++ {
		total += Backoff(attempt)
	}
	if Backoff(1) != time.Minute || Backoff(7) != 4*time.Hour || Backoff(20) != 4*time.Hour || total != 7*time.Hour+51*time.Minute {
		t.Errorf("backoff: %v %v total %v", Backoff(1), Backoff(7), total)
	}
}

func jsonInt(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
