// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"group24.de/itsm/test/load/internal/machine"
	"group24.de/itsm/test/load/internal/measure"
	"group24.de/itsm/test/load/internal/scenario"
)

// The flood stage (AP1, API plan §2.3): what happens when more comes than the server can take.
// Each case sends one kind of request at a fixed rate, whether the earlier ones were answered
// or not; beside it one normal person reads (LZ1) and writes (LZ4) as always. Not part of the
// CI: it measures how the server defends itself, not whether a change made it slower.

// floodOptions are the settings of one flood run.
type floodOptions struct {
	Rate     float64
	Duration time.Duration
	Pause    time.Duration
	Quiet    time.Duration
	Only     map[string]bool
	Out      string
}

// FloodCaseResult is one case of the report.
type FloodCaseResult struct {
	ID    string                    `json:"id"`
	Title string                    `json:"title"`
	Flood measure.RateResult        `json:"flood"`
	User  map[string]measure.Result `json:"user"`
}

// FloodReport is the report of a flood run (--out).
type FloodReport struct {
	Stage   string                    `json:"stage"`
	Machine machine.Machine           `json:"machine"`
	Rate    float64                   `json:"rate"`
	Seconds float64                   `json:"seconds"`
	Quiet   map[string]measure.Result `json:"quiet"`
	Cases   []FloodCaseResult         `json:"cases"`
	Started time.Time                 `json:"started"`
}

// userScenarios are what the normal person does beside the flood.
var userScenarios = []string{"LZ1", "LZ4"}

func runFlood(ctx context.Context, out io.Writer, base string, o floodOptions) error {
	setup := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 16}}
	// The flood keeps up to four times the rate open; the tool's own limits must not be the
	// bottleneck, so its pool is as large.
	open := int(4 * o.Rate)
	flood := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{
		MaxIdleConns: open, MaxIdleConnsPerHost: open, IdleConnTimeout: 30 * time.Second,
	}}

	// The person beside the flood, signed in before any case: the login flood locks the
	// address of the tool for 15 minutes (PL28), a later sign-in would fail.
	calls := map[string]measure.CallFunc{}
	var cleanups []func() (int, error)
	for _, s := range scenario.All() {
		for _, id := range userScenarios {
			if s.ID != id {
				continue
			}
			do, err := prepare(ctx, setup, s, base)
			if err != nil {
				return fmt.Errorf("%s vorbereiten: %w", id, err)
			}
			calls[id] = do
			if s.Cleanup != nil {
				c := s.Cleanup
				cleanups = append(cleanups, func() (int, error) { return c(ctx, setup, base) })
			}
		}
	}
	cases, removeClient, err := scenario.FloodSetup(ctx, setup, base)
	if err != nil {
		return fmt.Errorf("vorbereiten der Flut: %w", err)
	}
	defer func() {
		if err := removeClient(ctx); err != nil {
			fmt.Fprintf(out, "Zugang der Flut nicht entfernt: %v\n", err)
		}
		for _, c := range cleanups {
			if n, err := c(); err != nil {
				fmt.Fprintf(out, "Aufräumen: %v\n", err)
			} else if n > 0 {
				fmt.Fprintf(out, "Aufgeräumt: %d Datensätze aus LZ4\n", n)
			}
		}
	}()

	rep := FloodReport{Stage: "flood", Machine: machine.Detect(), Rate: o.Rate,
		Seconds: o.Duration.Seconds(), Started: time.Now().UTC()}
	rep.Quiet = besideFlood(ctx, setup, calls, 0, o.Quiet)
	fmt.Fprintf(out, "Ohne Flut, eine Person, je %s: %s\n", o.Quiet, userLine(rep.Quiet))

	for _, c := range cases {
		if len(o.Only) > 0 && !o.Only[c.ID] {
			continue
		}
		if err := waitReady(ctx, setup, base, time.Minute); err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
		fmt.Fprintf(out, "%-3s %s: %.0f je Sekunde für %s …\n", c.ID, c.Title, o.Rate, o.Duration)
		build := func(ctx context.Context) (*http.Request, error) { return c.Request(ctx, base) }
		var r measure.RateResult
		var user map[string]measure.Result
		var wg sync.WaitGroup
		wg.Go(func() { r = measure.RunRate(ctx, flood, build, o.Rate, o.Duration, open) })
		// The person starts two seconds later, when the flood has built up, and stops with it.
		wg.Go(func() { user = besideFlood(ctx, setup, calls, 2*time.Second, o.Duration-2*time.Second) })
		wg.Wait()
		rep.Cases = append(rep.Cases, FloodCaseResult{ID: c.ID, Title: c.Title, Flood: r, User: user})
		fmt.Fprintf(out, "    gesendet %d, übersprungen %d · 2xx %d · 401 %d · 429 %d · 503 %d · andere 4xx %d · andere 5xx %d · ohne Antwort %d\n",
			r.Sent, r.Skipped, r.OK, r.Status401, r.TooMany, r.Unavailable, r.Other4xx, r.Other5xx, r.Failed)
		fmt.Fprintf(out, "    Antworten der Flut: p50 %.1f ms, p95 %.1f ms, p99 %.1f ms, %.0f je Sekunde\n",
			r.P50ms, r.P95ms, r.P99ms, r.AnsweredPerSec)
		fmt.Fprintf(out, "    daneben eine Person: %s\n", userLine(user))
		if o.Pause > 0 {
			time.Sleep(o.Pause)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprint(out, floodTable(rep))
	if o.Out != "" {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(o.Out, raw, 0o644); err != nil { //nolint:gosec // G306: a report for the reader, no secret
			return err
		}
	}
	return nil
}

// besideFlood lets the person read and write at the same time, one call after the other each,
// for d after warmup.
func besideFlood(ctx context.Context, client *http.Client, calls map[string]measure.CallFunc, warmup, d time.Duration) map[string]measure.Result {
	if d < time.Second {
		d = time.Second
	}
	res := map[string]measure.Result{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range userScenarios {
		do, ok := calls[id]
		if !ok {
			continue
		}
		wg.Go(func() {
			r := measure.RunCalls(ctx, client, do, measure.Options{Workers: 1, Warmup: warmup, Duration: d})
			mu.Lock()
			res[id] = r
			mu.Unlock()
		})
	}
	wg.Wait()
	return res
}

func userLine(res map[string]measure.Result) string {
	var parts []string
	for _, id := range userScenarios {
		r, ok := res[id]
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s p95 %.1f ms (%d Aufrufe, %d Fehler)", id, r.P95ms, r.Requests, r.Errors))
	}
	return strings.Join(parts, " · ")
}

// floodTable is the report as a Markdown table for the plan.
func floodTable(rep FloodReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| Fall | gesendet | 2xx | 401 | 429 | 503 | andere | ohne Antwort | p95 Flut | LZ1 p95 | LZ4 p95 |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	q := rep.Quiet
	fmt.Fprintf(&b, "| ohne Flut | | | | | | | | | %s | %s |\n", userCell(q["LZ1"]), userCell(q["LZ4"]))
	for _, c := range rep.Cases {
		r := c.Flood
		sent := fmt.Sprintf("%d", r.Sent)
		if r.Skipped > 0 {
			sent += fmt.Sprintf(" (+%d übersprungen)", r.Skipped)
		}
		fmt.Fprintf(&b, "| %s %s | %s | %s | %s | %s | %s | %s | %s | %.1f ms | %s | %s |\n",
			c.ID, c.Title, sent, pct(r.OK, r.Sent), pct(r.Status401, r.Sent), pct(r.TooMany, r.Sent),
			pct(r.Unavailable, r.Sent), pct(r.Other4xx+r.Other5xx, r.Sent), pct(r.Failed, r.Sent),
			r.P95ms, userCell(c.User["LZ1"]), userCell(c.User["LZ4"]))
	}
	return b.String()
}

func userCell(r measure.Result) string {
	if r.Requests == 0 {
		return "–"
	}
	s := fmt.Sprintf("%.1f ms", r.P95ms)
	if r.Errors > 0 {
		s += fmt.Sprintf(", %d von %d Fehler", r.Errors, r.Requests)
	}
	return s
}

func pct(n, of int) string {
	if of == 0 || n == 0 {
		return "0"
	}
	return fmt.Sprintf("%.1f %%", 100*float64(n)/float64(of))
}

// waitReady waits until the server answers /readyz again after a case.
func waitReady(ctx context.Context, client *http.Client, base string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/readyz", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("der Server ist nach %s nicht bereit", limit)
		}
		time.Sleep(time.Second)
	}
}
