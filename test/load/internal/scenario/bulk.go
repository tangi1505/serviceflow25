// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// --- LZ5: throughput of the asynchronous bulk (PL32) ---
//
// Two NDJSON streams at once, three quarters probe.probe and a quarter core.person, with fixed
// keys under the source load-lz5 and values new in every run: the first run creates, every
// later one changes every row. The result is rows per second from the first byte sent until
// both jobs report done; a failed row or a failed job is an error of the run.

// bulkSource is the source of the LZ5 rows.
const bulkSource = "load-lz5"

// bulkWait is how long a run may take before it counts as failed; the fill in Prod size
// (SetBulkWait) waits longer for its larger streams.
var bulkWait = 5 * time.Minute

// SetBulkWait sets how long a bulk job may take (test/load fill).
func SetBulkWait(d time.Duration) { bulkWait = d }

// bulkRun sends rows rows and returns how many were written.
func bulkRun(ctx context.Context, client *http.Client, base string, rows int) (int, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return 0, err
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	persons := rows / 4
	streams := []struct {
		typ  string
		body []byte
	}{
		{probeType, ndjson(rows-persons, func(i int) map[string]any {
			return map[string]any{"external_key": fmt.Sprintf("lz5-p-%06d", i), "title": fmt.Sprintf("LZ5 Gerät %d %s", i, stamp), "priority": i%5 + 1}
		})},
		{"core.person", ndjson(persons, func(i int) map[string]any {
			return map[string]any{"external_key": fmt.Sprintf("lz5-q-%06d", i), "last_name": fmt.Sprintf("LZ5 Person %d", i), "first_name": stamp}
		})},
	}
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		total int
		errs  []error
	)
	for _, st := range streams {
		wg.Go(func() {
			n, err := s.bulkStream(ctx, client, base, st.typ, bulkSource, st.body)
			mu.Lock()
			defer mu.Unlock()
			total += n
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", st.typ, err))
			}
		})
	}
	wg.Wait()
	return total, errors.Join(errs...)
}

func ndjson(n int, row func(i int) map[string]any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := range n {
		_ = enc.Encode(row(i))
	}
	return buf.Bytes()
}

// bulkStream sends one stream and waits for its job.
func (s session) bulkStream(ctx context.Context, client *http.Client, base, typ, source string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/records/"+typ+":bulk?source="+source, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	s.apply(req)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return 0, fmt.Errorf("stream: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	location := resp.Header.Get("Location")
	deadline := time.Now().Add(bulkWait)
	for time.Now().Before(deadline) {
		var job struct {
			State  string `json:"state"`
			Output struct {
				Done      bool `json:"done"`
				Created   int  `json:"created"`
				Updated   int  `json:"updated"`
				Unchanged int  `json:"unchanged"`
				Errors    int  `json:"errors"`
			} `json:"output"`
			Errors []json.RawMessage `json:"errors"`
		}
		if err := s.getJSON(ctx, client, base+location, &job); err != nil {
			return 0, err
		}
		out := job.Output
		switch {
		case job.State == "completed" && out.Done:
			n := out.Created + out.Updated + out.Unchanged
			if out.Errors > 0 {
				return n, fmt.Errorf("%d rows failed", out.Errors)
			}
			return n, nil
		case job.State == "discarded" || job.State == "cancelled" || len(job.Errors) > 0:
			first := ""
			if len(job.Errors) > 0 {
				first = ": " + string(job.Errors[len(job.Errors)-1])
			}
			return 0, fmt.Errorf("job %s: %d errors%s", job.State, len(job.Errors), first)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0, fmt.Errorf("job %s not done after %s", location, bulkWait)
}

func (s session) getJSON(ctx context.Context, client *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	s.apply(req)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s: %s", url, resp.Status, strings.TrimSpace(string(raw)))
	}
	return json.Unmarshal(raw, v)
}

// --- LZ13: the dispatcher behind a bulk run (PL41) ---

// dispatchRun is a bulk run like LZ5 and then the wait until every durable consumer of the
// dispatcher (live, webhooks, rules) has taken the newest event. The result is rows per
// second over the whole of it: every row is at least one event, so this is the rate at which
// the events of a bulk run reach webhooks and rules.
func dispatchRun(ctx context.Context, client *http.Client, base string, rows int) (int, error) {
	n, err := bulkRun(ctx, client, base, rows)
	if err != nil {
		return n, err
	}
	s, err := login(ctx, client, base)
	if err != nil {
		return n, err
	}
	deadline := time.Now().Add(bulkWait)
	for {
		var list []struct {
			Name    string `json:"name"`
			Lag     int64  `json:"lag"`
			Durable bool   `json:"durable"`
		}
		if err := s.getJSON(ctx, client, base+"/api/v1/events/consumers", &list); err != nil {
			return n, err
		}
		behind := ""
		for _, c := range list {
			if c.Durable && c.Lag > 0 {
				behind = fmt.Sprintf("%s %d behind", c.Name, c.Lag)
			}
		}
		if behind == "" {
			return n, nil
		}
		if time.Now().After(deadline) {
			return n, fmt.Errorf("the dispatcher did not catch up in %s: %s", bulkWait, behind)
		}
		select {
		case <-ctx.Done():
			return n, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
