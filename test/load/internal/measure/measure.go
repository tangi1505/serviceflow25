// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package measure runs a closed-model load against one HTTP request and reports latency
// percentiles and throughput (Hauptplan H11, E51).
package measure

import (
	"context"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"
)

// RequestFunc builds the request of one call.
type RequestFunc func(ctx context.Context) (*http.Request, error)

// Options control one measurement.
type Options struct {
	Workers  int
	Warmup   time.Duration
	Duration time.Duration
}

// Result of one measurement. Durations are in milliseconds for readable JSON. Unavailable
// is set instead of numbers when the target could not be prepared for the scenario (a baseline
// build without the endpoint): nothing was measured then.
type Result struct {
	Unavailable string  `json:"unavailable,omitempty"`
	Requests    int     `json:"requests"`
	Errors      int     `json:"errors"`
	P50ms       float64 `json:"p50_ms"`
	P95ms       float64 `json:"p95_ms"`
	P99ms       float64 `json:"p99_ms"`
	RPS         float64 `json:"rps"`
	// Rows is set for a throughput run (LZ5): rows written; RPS is then rows per second.
	Rows int `json:"rows,omitempty"`
}

// Run measures until o.Duration has passed after the warmup. Calls during the warmup are not
// counted. A call counts as an error when it fails or answers outside 2xx.
func Run(ctx context.Context, client *http.Client, req RequestFunc, o Options) Result {
	if o.Workers < 1 {
		o.Workers = 1
	}
	start := time.Now()
	measureFrom := start.Add(o.Warmup)
	end := measureFrom.Add(o.Duration)

	var mu sync.Mutex
	var latencies []time.Duration
	errors := 0

	var wg sync.WaitGroup
	for range o.Workers {
		wg.Go(func() {
			var local []time.Duration
			localErr := 0
			for ctx.Err() == nil {
				t0 := time.Now()
				if !t0.Before(end) {
					break
				}
				ok := call(ctx, client, req)
				d := time.Since(t0)
				if t0.Before(measureFrom) {
					continue
				}
				if ok {
					local = append(local, d)
				} else {
					localErr++
				}
			}
			mu.Lock()
			latencies = append(latencies, local...)
			errors += localErr
			mu.Unlock()
		})
	}
	wg.Wait()

	r := Result{Requests: len(latencies) + errors, Errors: errors}
	if o.Duration > 0 {
		r.RPS = float64(len(latencies)) / o.Duration.Seconds()
	}
	slices.Sort(latencies)
	r.P50ms = ms(Percentile(latencies, 50))
	r.P95ms = ms(Percentile(latencies, 95))
	r.P99ms = ms(Percentile(latencies, 99))
	return r
}

func call(ctx context.Context, client *http.Client, build RequestFunc) bool {
	req, err := build(ctx)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// Percentile returns the nearest-rank percentile of sorted values; 0 for no values.
func Percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(p/100*float64(len(sorted)) + 0.999999)
	rank = max(1, min(rank, len(sorted)))
	return sorted[rank-1]
}

// Best of several repeated runs: the result with the smallest p95 (Hauptplan E76). Disturbance
// on a shared machine only ever slows a run down, so the best run is the closest to the cost
// of the code itself; the median still carried the noise of cooling down after the test phase.
func Best(results []Result) Result {
	if len(results) == 0 {
		return Result{}
	}
	return slices.MinFunc(results, func(a, b Result) int {
		switch {
		case a.P95ms < b.P95ms:
			return -1
		case a.P95ms > b.P95ms:
			return 1
		}
		return 0
	})
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}
