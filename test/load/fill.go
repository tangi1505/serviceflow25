// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"group24.de/itsm/test/load/internal/gen"
	"group24.de/itsm/test/load/internal/scenario"
)

// --- fill: the data set in Prod size (H14, Hauptplan E128, §2.5) ---
//
//	go run ./test/load fill --size=prod --target=http://127.0.0.1:18080 --database=postgres://…
//
// Persons (core.person) and incidents (incident.incident, packages sd and incident) come over
// the bulk API like in operation, in streams of --chunk rows, and the time of each is
// reported (your decision 2026-11-13). The bulk rightly sets neither the moment of creation
// nor the state, so a last step of this tool only gives the incidents their ten years of
// history directly in the database: created evenly over ten years up to now, open only in the
// last 30 days, older ones closed, the resolved and closed with "resolved at" and both
// durations, their SLA clocks stopped. Computers wait for the asset package (Meilenstein 2).
// No real data (VS-NfD): names, titles and addresses are made up.

const fillSource = "loadgen"

// fillWords make the titles; LZ3 searches for them.
var fillWords = []string{"Drucker", "Anmeldung", "Netzwerk", "Laufwerk", "Monitor", "Kennwort", "VPN", "Mail", "Kalender",
	"Software", "Zugang", "Freigabe", "Telefon", "Scanner", "Zertifikat", "Update", "Lizenz", "Speicher"}

var fillTrouble = []string{"fällt aus", "langsam", "defekt", "meldet Fehler", "reagiert nicht", "ohne Verbindung"}

var levels = []string{"low", "medium", "high"}

func cmdFill(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("fill", flag.ContinueOnError)
	size := fs.String("size", "ci", "tiny, ci or prod")
	target := fs.String("target", "", "base URL of the instance")
	dbURL := fs.String("database", "", "the database of the instance, for the history step")
	chunk := fs.Int("chunk", 50_000, "rows per bulk stream")
	steps := fs.String("steps", "persons,teams,incidents,history", "which steps, comma-separated")
	from := fs.Int("from", 0, "first incident (to go on after a stop)")
	parallel := fs.Int("parallel", 1, "streams at once; the instance needs as many bulk workers (ITSM_WORKERS_BULK)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, ok := gen.Sizes[*size]
	if !ok || *target == "" || *dbURL == "" {
		return errors.New("fill needs --size=tiny|ci|prod, --target=URL and --database=URL")
	}
	ctx := context.Background()
	db, err := pgx.Connect(ctx, *dbURL)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	client := &http.Client{Timeout: 10 * time.Minute}
	scenario.SetBulkWait(2 * time.Hour)
	want := map[string]bool{}
	for _, st := range strings.Split(*steps, ",") {
		want[strings.TrimSpace(st)] = true
	}
	stream := func(typ, what string, n, first int, row func(i int) map[string]any) error {
		start := time.Now()
		var (
			mu   sync.Mutex
			done int
			errs []error
		)
		chunks := make(chan int)
		var wg sync.WaitGroup
		for range max(1, *parallel) {
			wg.Go(func() {
				for lo := range chunks {
					hi := min(n, lo+*chunk)
					var buf bytes.Buffer
					enc := json.NewEncoder(&buf)
					for i := lo; i < hi; i++ {
						_ = enc.Encode(row(i))
					}
					t0 := time.Now()
					got, err := scenario.Bulk(ctx, client, *target, typ, fillSource, buf.Bytes())
					mu.Lock()
					if err != nil {
						errs = append(errs, fmt.Errorf("%s %d bis %d: %w", what, lo, hi, err))
					}
					done += got
					el := time.Since(start)
					fmt.Fprintf(out, "%s %9d bis %9d: %6.0f Zeilen/s, gesamt %9d in %s (%.0f/s)\n", what, lo, hi,
						float64(got)/time.Since(t0).Seconds(), done, el.Round(time.Second), float64(done)/el.Seconds())
					mu.Unlock()
				}
			})
		}
		for lo := first; lo < n; lo += *chunk {
			mu.Lock()
			failed := len(errs) > 0
			mu.Unlock()
			if failed {
				break
			}
			chunks <- lo
		}
		close(chunks)
		wg.Wait()
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		fmt.Fprintf(out, "%s fertig: %d Zeilen in %s (%.0f/s)\n", what, done, time.Since(start).Round(time.Second), float64(done)/time.Since(start).Seconds())
		return nil
	}

	if want["persons"] {
		err := stream("core.person", "Personen", s.Persons, 0, func(i int) map[string]any {
			return map[string]any{"external_key": personKey(i), "first_name": "Lasttest",
				"last_name": fmt.Sprintf("Person %06d", i), "email": fmt.Sprintf("lasttest.%06d@example.invalid", i)}
		})
		if err != nil {
			return err
		}
	}
	if want["teams"] {
		// The teams the incidents go to (LZ2: the queue of a team, scenario.ProdTeam).
		err := stream("core.group", "Teams", fillTeams, 0, func(i int) map[string]any {
			return map[string]any{"external_key": fmt.Sprintf("team-%02d", i), "name": fmt.Sprintf("Lasttest Team %02d", i)}
		})
		if err != nil {
			return err
		}
	}
	if want["incidents"] {
		teams, err := idsOf(ctx, db, "rec_core_group")
		if err != nil {
			return err
		}
		if len(teams) != fillTeams {
			return fmt.Errorf("fill: %d teams of the source loadgen, want %d; run the step teams first", len(teams), fillTeams)
		}
		persons, err := personIDs(ctx, db)
		if err != nil {
			return err
		}
		if len(persons) == 0 {
			return errors.New("fill: no persons of the source loadgen; run the step persons first")
		}
		err = stream("incident.incident", "Störungen", s.Tickets, *from, func(i int) map[string]any {
			h := mix(uint64(i)) //nolint:gosec // a row number, never negative
			return map[string]any{
				"external_key": ticketKey(i),
				"title":        fillWords[h%uint64(len(fillWords))] + " " + fillTrouble[(h>>8)%uint64(len(fillTrouble))],
				"description":  fmt.Sprintf("Lasttest-Störung %d.", i),
				"impact":       levels[(h>>16)%3], "urgency": levels[(h>>24)%3],
				"actors": map[string]any{
					"requester": []any{map[string]any{"type": "core.person", "id": persons[int((h>>32)%uint64(len(persons)))]}}, //nolint:gosec // below len(persons)
					"group":     []any{map[string]any{"type": "core.group", "id": teams[i%fillTeams]}},
				},
			}
		})
		if err != nil {
			return err
		}
	}
	if want["history"] {
		return history(ctx, db, s.Tickets, out)
	}
	return nil
}

func personKey(i int) string { return fmt.Sprintf("person-%08d", i) }
func ticketKey(i int) string { return fmt.Sprintf("ticket-%08d", i) }

// mix is a fixed hash of the row number: the same data on every run.
func mix(x uint64) uint64 {
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}

// fillTeams is the number of teams the incidents are spread over.
const fillTeams = 50

func personIDs(ctx context.Context, db *pgx.Conn) ([]string, error) {
	return idsOf(ctx, db, "rec_core_person")
}

// idsOf lists the ids of the records of the source in a table, in the order of their keys.
func idsOf(ctx context.Context, db *pgx.Conn, table string) ([]string, error) {
	rows, err := db.Query(ctx, "SELECT id::text FROM "+pgx.Identifier{table}.Sanitize()+" WHERE source = $1 ORDER BY external_key", fillSource)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// history gives the incidents of the source their ten years, a million rows at a time: the
// moment of creation evenly up to now, open states only in the last 30 days (by a hash of the
// key, not the row number, which also picks the team: 2 in 10 new, 3 in progress, 1 waiting, 2
// resolved, 2 closed), closed before; resolved after 1 to 72 hours, in service time 40 % of
// it; the clocks of the done ones stopped at the resolution, those of the open ones running.
// Last it removes the jobs the import queued (deadlines, notifications, rules). It can run
// again on a filled database.
func history(ctx context.Context, db *pgx.Conn, n int, out io.Writer) error {
	start := time.Now()
	const step = 1_000_000
	for lo := 0; lo < n; lo += step {
		hi := min(n, lo+step)
		t0 := time.Now()
		tag, err := db.Exec(ctx, `
WITH h AS (
  SELECT r.id, x.created,
         CASE WHEN x.created < now() - interval '30 days' THEN 'closed'
              ELSE (ARRAY['new','new','in_progress','in_progress','in_progress','waiting','resolved','resolved','closed','closed'])[1 + (abs(hashtext(r.external_key)) % 10)]
         END AS state,
         1 + (x.idx % 72) AS hours
  FROM rec_incident_incident r,
       LATERAL (SELECT substr(r.external_key, 8)::bigint AS idx) i,
       LATERAL (SELECT i.idx, now() - interval '10 years' + (i.idx::float8 / $3) * (interval '10 years') AS created) x
  WHERE r.source = $4 AND r.external_key >= $1 AND r.external_key < $2
), d AS (
  SELECT h.*, CASE WHEN h.state IN ('resolved', 'closed') THEN least(h.created + h.hours * interval '1 hour', now()) END AS resolved
  FROM h
)
UPDATE rec_incident_incident r SET
  created_at = d.created, updated_at = coalesce(d.resolved, d.created), state = d.state,
  resolved_at = d.resolved,
  resolution_hours = CASE WHEN d.resolved IS NOT NULL THEN d.hours END,
  resolution_service_hours = CASE WHEN d.resolved IS NOT NULL THEN round(d.hours * 0.4, 2) END,
  sla_due_at = CASE WHEN d.resolved IS NULL THEN coalesce(r.sla_due_at, d.created + interval '8 hours') END,
  sla_warn_at = CASE WHEN d.resolved IS NULL THEN coalesce(r.sla_warn_at, d.created + interval '6 hours') END
FROM d WHERE r.id = d.id`, ticketKey(lo), ticketKey(hi), n, fillSource)
		if err != nil {
			return fmt.Errorf("history %d bis %d: %w", lo, hi, err)
		}
		if _, err := db.Exec(ctx, `
UPDATE clock c SET
  state = CASE WHEN r.resolved_at IS NULL THEN 'running' ELSE 'stopped' END,
  stopped_at = r.resolved_at, started_at = r.created_at, updated_at = now(),
  resumed_at = CASE WHEN r.resolved_at IS NULL THEN r.created_at END,
  due_at = CASE WHEN r.resolved_at IS NULL THEN r.created_at + c.target_seconds * interval '1 second' ELSE c.due_at END
FROM rec_incident_incident r
WHERE c.record_id = r.id AND c.type_name = 'incident.incident'
  AND r.source = $3 AND r.external_key >= $1 AND r.external_key < $2`, ticketKey(lo), ticketKey(hi), fillSource); err != nil {
			return fmt.Errorf("clocks %d bis %d: %w", lo, hi, err)
		}
		fmt.Fprintf(out, "Geschichte %9d bis %9d: %d Zeilen in %s\n", lo, hi, tag.RowsAffected(), time.Since(t0).Round(time.Second))
	}
	// The import queued deadlines, notifications and rules for every incident; after ten years
	// of operation those of the history are long done. They go, so that the measurement sees
	// the everyday state (your decision 2026-11-14, H14).
	t0 := time.Now()
	tag, err := db.Exec(ctx, `DELETE FROM river_job WHERE state IN ('available', 'scheduled', 'retryable')
  AND kind IN ('sla.due', 'sla.warn', 'rules.apply', 'notify.fanout', 'notify.send')`)
	if err != nil {
		return fmt.Errorf("jobs of the import: %w", err)
	}
	fmt.Fprintf(out, "Jobs des Imports entfernt: %d in %s\n", tag.RowsAffected(), time.Since(t0).Round(time.Second))
	fmt.Fprintf(out, "Geschichte fertig in %s\n", time.Since(start).Round(time.Second))
	return nil
}
