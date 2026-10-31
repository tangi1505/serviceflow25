// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package events

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/schema"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

const typeName = "eventstest.thing"

// setup installs a package of its own and returns a service with Valkey. Other packages test
// against the same database at the same time (and the API tests empty the probe table), so
// the type is this package's and every check filters by its own records.
func setup(t *testing.T) (context.Context, *Service, *records.Service, *pgxpool.Pool) {
	t.Helper()
	url, addr := os.Getenv("ITSM_TEST_DATABASE_URL"), os.Getenv("ITSM_TEST_VALKEY_ADDR")
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
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml": {Data: []byte("name: eventstest\nversion: 0.1.0\nlabel: {de: T, en: T}\n")},
		"types/thing.yaml": {Data: []byte(`name: thing
label: {de: Ding, en: Thing}
fields:
  - {name: title, type: text, label: {de: T, en: T}, required: true, max_length: 50}
  - {name: priority, type: number, label: {de: P, en: P}, min: 1, max: 5, default: 3}
states:
  - {name: new, label: {de: Neu, en: New}, initial: true}
  - {name: in_progress, label: {de: In Arbeit, en: In progress}}
transitions:
  - {name: start, label: {de: Beginnen, en: Start}, from: [new], to: in_progress}
`)},
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
	kv, err := cache.Open(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(kv.Close)
	rec := records.New(db.Single(pool))
	return ctx, New(db.Single(pool), rec, kv, quiet), rec, pool
}

// ownDatabase gives a test a database of its own: it deletes the start of the event log, which
// the tests of other packages running at the same time still read (their events would vanish,
// and theirs would hold its own deletion back).
func ownDatabase(t *testing.T, name string) {
	t.Helper()
	if base := os.Getenv("ITSM_TEST_DATABASE_URL"); base != "" {
		t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, name))
	}
}

func create(t *testing.T, ctx context.Context, rec *records.Service, title string) uuid.UUID {
	t.Helper()
	r, err := rec.Create(ctx, typeName, map[string]any{"title": title}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return uuid.MustParse(r["id"].(string))
}

// ofRecord keeps the events of one record, in order.
func ofRecord(list []records.Event, id uuid.UUID) []records.Event {
	var out []records.Event
	for _, e := range list {
		if e.RecordID == id {
			out = append(out, e)
		}
	}
	return out
}

// readAll reads every event after a position.
func readAll(t *testing.T, ctx context.Context, s *Service, after int64) []records.Event {
	t.Helper()
	var out []records.Event
	for {
		list, last, more, err := s.Stream(ctx, after, 1000)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, list...)
		after = last
		if !more {
			return out
		}
	}
}

// Twenty concurrent writers: every event appears, positions strictly increase, and the
// stream never shows an event behind one still in progress (E87): an event of an open
// transaction holds everything after it back until it commits, then both appear in order.
func TestConcurrentWritersAndHorizon(t *testing.T) {
	ctx, s, rec, pool := setup(t)
	start, err := s.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	mine := map[uuid.UUID]bool{}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			r, err := rec.Create(ctx, typeName, map[string]any{"title": "parallel", "priority": i%5 + 1}, nil)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			mine[uuid.MustParse(r["id"].(string))] = true
			mu.Unlock()
		})
	}
	wg.Wait()
	// Open transactions of other test packages on the same database hold the stream back for
	// a moment (the horizon, E87), so the events may need a few reads to appear.
	var all []records.Event
	seen := 0
	after := start
	for deadline := time.Now().Add(10 * time.Second); seen < 20 && time.Now().Before(deadline); {
		batch := readAll(t, ctx, s, after)
		for _, e := range batch {
			if n := len(all); n > 0 && e.Position <= all[n-1].Position {
				t.Fatalf("disorder at %d: position %d", n, e.Position)
			}
			all = append(all, e)
			if mine[e.RecordID] {
				seen++
			}
		}
		if len(batch) > 0 {
			after = batch[len(batch)-1].Position
		}
		if seen < 20 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if seen != 20 {
		t.Fatalf("20 own events expected after %d, found %d of %d", start, seen, len(all))
	}

	// The horizon: an event inserted in an open transaction takes a position; a record
	// created and committed afterwards stays invisible until that transaction commits.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var held int64
	heldID := uuid.New()
	if err := tx.QueryRow(ctx, "INSERT INTO event (kind, type_name, record_id, payload) VALUES ('create', $1, $2, '{}') RETURNING position", typeName, heldID).Scan(&held); err != nil {
		t.Fatal(err)
	}
	later := create(t, ctx, rec, "behind the open transaction")
	for _, e := range readAll(t, ctx, s, start) {
		if e.Position >= held {
			t.Fatalf("event %d (%s) shown although %d is still open", e.Position, e.Kind, held)
		}
	}
	if latest, _ := s.Latest(ctx); latest >= held {
		t.Fatalf("latest %d must stay below the open event %d", latest, held)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got []int64
		for _, e := range readAll(t, ctx, s, held-1) {
			if e.RecordID == heldID || e.RecordID == later {
				got = append(got, e.Position)
			}
		}
		if len(got) == 2 && got[0] == held && got[1] > held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after the commit both events must show in order: %v", got)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Every change is one event with version and changed fields; the dispatcher publishes them
// in order and stores its position; a subscriber reads after any position; too old is gone.
func TestStreamDispatchAndRetention(t *testing.T) {
	ownDatabase(t, "itsmtest_events_retention")
	ctx, s, rec, pool := setup(t)
	start, _ := s.Latest(ctx)
	received := make(chan records.Event, 1024)
	subCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() { _ = s.Subscribe(subCtx, func(e records.Event) { received <- e }) }()
	time.Sleep(200 * time.Millisecond) // the subscription is up

	id := create(t, ctx, rec, "stream")
	if _, err := rec.Update(ctx, typeName, id, 1, map[string]any{"title": "stream 2", "priority": 4}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Transition(ctx, typeName, id, "start", nil, -1, nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.Trash(ctx, typeName, id, nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.Restore(ctx, typeName, id, nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.Trash(ctx, typeName, id, nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.Purge(ctx, typeName, id, nil); err != nil {
		t.Fatal(err)
	}
	kinds := []string{"create", "update", "transition", "trash", "restore", "trash", "purge"}
	// An open transaction elsewhere (a bulk batch of another package's tests, PL32) holds the
	// horizon back for a moment (E87): wait until the events are visible. On a busy machine
	// with -race such a batch took longer than ten seconds (PL47d), hence a minute at most.
	var list []records.Event
	for deadline := time.Now().Add(time.Minute); ; time.Sleep(50 * time.Millisecond) {
		list = ofRecord(readAll(t, ctx, s, start), id)
		if len(list) >= len(kinds) || time.Now().After(deadline) {
			break
		}
	}
	if len(list) != len(kinds) {
		t.Fatalf("events of the record: %d", len(list))
	}
	for i, e := range list {
		var payload struct {
			Version int64    `json:"version"`
			Changed []string `json:"changed"`
			OrgPath string   `json:"org_path"`
			To      string   `json:"to"`
		}
		_ = json.Unmarshal(e.Payload, &payload)
		if e.Kind != kinds[i] || payload.OrgPath != "root" || payload.Changed == nil || (i > 0 && e.Position <= list[i-1].Position) {
			t.Fatalf("event %d: %s %s", i, e.Kind, e.Payload)
		}
		switch e.Kind {
		case "update":
			if payload.Version != 2 || len(payload.Changed) != 2 {
				t.Fatalf("update event: %s", e.Payload)
			}
		case "transition":
			if payload.Version != 3 || payload.To != "in_progress" || payload.Changed[0] != "state" {
				t.Fatalf("transition event: %s", e.Payload)
			}
		case "purge":
			if payload.Version != 6 {
				t.Fatalf("purge event: %s", e.Payload)
			}
		}
	}
	end := list[6].Position

	// The consumer live publishes everything after its position, in order; the test's own row
	// (PL41) does not hold the retention back.
	s.Prefix = "eventstest-"
	if _, err := pool.Exec(ctx, `INSERT INTO event_consumer (name, position, durable) VALUES ('eventstest-live', $1, false)
		ON CONFLICT (name) DO UPDATE SET position = $1`, start); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if p := s.Position(ctx, ConsumerLive); p < end {
		t.Fatalf("dispatcher position %d, want at least %d", p, end)
	}
	// Delivery is at least once: another dispatcher (the API tests run one) may publish the
	// same events too, so duplicates of a position are dropped.
	var published []records.Event
	seenPos := map[int64]bool{}
	deadline := time.After(5 * time.Second)
	for len(published) < len(kinds) {
		select {
		case e := <-received:
			if e.RecordID == id && !seenPos[e.Position] {
				seenPos[e.Position] = true
				published = append(published, e)
			}
		case <-deadline:
			t.Fatalf("only %d of %d events published", len(published), len(kinds))
		}
	}
	for i, e := range published {
		if e.Position != list[i].Position || e.Kind != kinds[i] {
			t.Fatalf("published %d: position %d kind %s", i, e.Position, e.Kind)
		}
	}

	// A subscriber that stopped at the fifth event gets the sixth and seventh.
	fifth := list[4].Position
	if got := ofRecord(readAll(t, ctx, s, fifth), id); len(got) != 2 || got[0].Kind != "trash" || got[1].Kind != "purge" {
		t.Fatalf("catch up: %v", got)
	}
	// Retention deleted everything up to the fifth: a position before it is gone, the fifth
	// itself still works (in the database of this test, see ownDatabase).
	if _, err := pool.Exec(ctx, "DELETE FROM event WHERE position <= $1", fifth); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Stream(ctx, fifth-2, 100); !errors.Is(err, ErrGone) {
		t.Fatalf("too old must be gone: %v", err)
	}
	if got := ofRecord(readAll(t, ctx, s, fifth), id); len(got) != 2 {
		t.Fatalf("exactly at the oldest kept: %v", got)
	}
}
