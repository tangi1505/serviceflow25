// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package measure

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// RateResult is one measurement at a fixed rate (AP1, flood): requests go out on a schedule,
// whether the earlier ones have been answered or not, as a flood does. The answers are counted
// by kind instead of as errors: a 429 or a 503 is the server defending itself.
type RateResult struct {
	// Rate is the rate asked for, per second; Sent how many went out in the time.
	Rate float64 `json:"rate"`
	Sent int     `json:"sent"`
	// Skipped were due but not sent, because MaxInFlight requests were still open: then the
	// server answers slower than the flood comes, and the tool does not pile up without end.
	Skipped     int `json:"skipped"`
	OK          int `json:"status_2xx"`
	Status401   int `json:"status_401"`
	Other4xx    int `json:"status_4xx_other"`
	TooMany     int `json:"status_429"`
	Unavailable int `json:"status_503"`
	Other5xx    int `json:"status_5xx_other"`
	// Failed got no answer: a timeout of the tool or a broken connection.
	Failed int `json:"failed"`
	// Times of the answered requests, whatever their status.
	P50ms float64 `json:"p50_ms"`
	P95ms float64 `json:"p95_ms"`
	P99ms float64 `json:"p99_ms"`
	// Answered per second within the time.
	AnsweredPerSec float64 `json:"answered_per_s"`
}

// tick is how often the schedule is checked; at 1,000 per second ten requests go out a tick.
const tick = 10 * time.Millisecond

// RunRate sends build at rate per second for d and waits for the answers still open after
// that (at most the client's timeout). maxInFlight bounds the open requests; 0 means 4 × rate.
func RunRate(ctx context.Context, client *http.Client, build RequestFunc, rate float64, d time.Duration, maxInFlight int) RateResult {
	if maxInFlight <= 0 {
		maxInFlight = int(4 * rate)
		if maxInFlight < 1 {
			maxInFlight = 1
		}
	}
	r := RateResult{Rate: rate}
	var mu sync.Mutex
	var latencies []time.Duration
	var open atomic.Int64
	var wg sync.WaitGroup

	start := time.Now()
	end := start.Add(d)
	due := 0
	t := time.NewTicker(tick)
	defer t.Stop()
	for now := start; now.Before(end) && ctx.Err() == nil; now = <-t.C {
		want := int(now.Sub(start).Seconds()*rate) + 1
		for ; due < want; due++ {
			if open.Load() >= int64(maxInFlight) {
				r.Skipped++
				continue
			}
			r.Sent++
			open.Add(1)
			wg.Go(func() {
				defer open.Add(-1)
				t0 := time.Now()
				code, err := send(ctx, client, build)
				took := time.Since(t0)
				mu.Lock()
				defer mu.Unlock()
				r.count(code, err)
				if err == nil {
					latencies = append(latencies, took)
				}
			})
		}
	}
	wg.Wait()
	slices.Sort(latencies)
	r.P50ms = ms(Percentile(latencies, 50))
	r.P95ms = ms(Percentile(latencies, 95))
	r.P99ms = ms(Percentile(latencies, 99))
	if d > 0 {
		r.AnsweredPerSec = float64(len(latencies)) / d.Seconds()
	}
	return r
}

// count files one answer under its kind.
func (r *RateResult) count(code int, err error) {
	switch {
	case err != nil:
		r.Failed++
	case code >= 200 && code < 300:
		r.OK++
	case code == http.StatusUnauthorized:
		r.Status401++
	case code == http.StatusTooManyRequests:
		r.TooMany++
	case code == http.StatusServiceUnavailable:
		r.Unavailable++
	case code >= 500:
		r.Other5xx++
	default:
		r.Other4xx++
	}
}

// send makes one request and returns its status; the body is read to the end so that the
// connection can be used again.
func send(ctx context.Context, client *http.Client, build RequestFunc) (int, error) {
	req, err := build(ctx)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode == 0 {
		return 0, errors.New("no status")
	}
	return resp.StatusCode, nil
}
