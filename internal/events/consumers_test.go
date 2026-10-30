// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package events

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/records"
)

// The dispatcher in batches (PL41): a consumer that fails keeps its position and its error
// and gets the same events again, while the others go on; what it wrote in the failed pass is
// gone with it; a consumer another instance holds is left alone; a batch is at most 1,000.
func TestConsumersKeepTheirPositions(t *testing.T) {
	ctx, s, rec, pool := setup(t)
	s.Prefix = "eventstest-pl41-"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM event_consumer WHERE name LIKE 'eventstest-pl41-%'")
	})
	if _, err := pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS eventstest_written (position bigint)"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE eventstest_written"); err != nil {
		t.Fatal(err)
	}
	fail := true
	var flakySaw, steadySaw []int64
	biggest := 0
	s.Consume("flaky", func(ctx context.Context, tx pgx.Tx, batch []records.Event) error {
		for _, e := range batch {
			if _, err := tx.Exec(ctx, "INSERT INTO eventstest_written VALUES ($1)", e.Position); err != nil {
				return err
			}
			flakySaw = append(flakySaw, e.Position)
		}
		if fail {
			return errors.New("receiver down")
		}
		return nil
	})
	s.Consume("steady", func(_ context.Context, _ pgx.Tx, batch []records.Event) error {
		biggest = max(biggest, len(batch))
		for _, e := range batch {
			steadySaw = append(steadySaw, e.Position)
		}
		return nil
	})
	if err := s.SkipToEnd(ctx); err != nil {
		t.Fatal(err)
	}
	start := s.Position(ctx, "flaky")
	id := create(t, ctx, rec, "pl41 one")
	create(t, ctx, rec, "pl41 two")

	// The flaky one fails: position kept, error kept, its writes rolled back; steady goes on.
	if err := s.Dispatch(ctx); err == nil {
		t.Fatal("the failure of a consumer must come back")
	}
	if p := s.Position(ctx, "flaky"); p != start {
		t.Fatalf("flaky moved to %d from %d", p, start)
	}
	if p := s.Position(ctx, "steady"); p <= start {
		t.Fatalf("steady stands at %d", p)
	}
	var written int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM eventstest_written").Scan(&written)
	if written != 0 {
		t.Fatalf("the failed pass left %d rows", written)
	}
	status := map[string]ConsumerStatus{}
	list, err := s.Consumers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		status[c.Name] = c
	}
	flaky := status["eventstest-pl41-flaky"]
	if flaky.LastError != "receiver down" || flaky.FailingSince == nil || flaky.Attempts != 1 || flaky.Lag < 2 || flaky.Durable {
		t.Fatalf("status of flaky: %+v", flaky)
	}

	// Next pass: the same events again, then the error is gone.
	fail = false
	flakySaw = nil
	if err := s.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(flakySaw) < 2 || s.Position(ctx, "flaky") != s.Position(ctx, "steady") {
		t.Fatalf("flaky after the retry: saw %v, at %d", flakySaw, s.Position(ctx, "flaky"))
	}
	list, _ = s.Consumers(ctx)
	for _, c := range list {
		if c.Name == "eventstest-pl41-flaky" && (c.LastError != "" || c.FailingSince != nil || c.Attempts != 0) {
			t.Fatalf("error not cleared: %+v", c)
		}
	}

	// Another instance holds the row: this one leaves the consumer alone.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT 1 FROM event_consumer WHERE name = 'eventstest-pl41-steady' FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	before := s.Position(ctx, "steady")
	if _, err := rec.Update(ctx, typeName, id, 1, map[string]any{"title": "pl41 one b"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if p := s.Position(ctx, "steady"); p != before {
		t.Fatalf("a held consumer moved: %d → %d", before, p)
	}
	_ = tx.Rollback(ctx)
	if err := s.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if p := s.Position(ctx, "steady"); p == before {
		t.Fatal("the released consumer did not go on")
	}
	if biggest > batch {
		t.Fatalf("a batch of %d", biggest)
	}
}
