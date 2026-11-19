// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package measure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunRateKeepsTheRateAndSortsTheAnswers: at 200 per second for a second about 200 requests
// go out, and every answer lands in its column (AP1).
func TestRunRateKeepsTheRateAndSortsTheAnswers(t *testing.T) {
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch i := n.Add(1); {
		case i%5 == 0:
			w.WriteHeader(http.StatusServiceUnavailable)
		case i%4 == 0:
			w.WriteHeader(http.StatusTooManyRequests)
		case i%7 == 0:
			w.WriteHeader(http.StatusUnauthorized)
		case i%11 == 0:
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	build := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	}
	r := RunRate(context.Background(), srv.Client(), build, 200, time.Second, 0)
	if r.Sent < 180 || r.Sent > 202 {
		t.Fatalf("sent %d at 200/s for 1 s", r.Sent)
	}
	sum := r.OK + r.Status401 + r.Other4xx + r.TooMany + r.Unavailable + r.Other5xx + r.Failed
	if sum != r.Sent || r.Skipped != 0 {
		t.Fatalf("answers %d of %d sent, skipped %d: %+v", sum, r.Sent, r.Skipped, r)
	}
	if r.OK == 0 || r.TooMany == 0 || r.Unavailable == 0 || r.Status401 == 0 || r.Other4xx == 0 {
		t.Fatalf("every kind should appear: %+v", r)
	}
	if r.P95ms <= 0 || r.AnsweredPerSec < 150 {
		t.Fatalf("times or answered rate missing: %+v", r)
	}
}

// TestRunRateSkipsWhenTooManyAreOpen: a server slower than the flood leaves requests open; past
// the bound the tool skips instead of piling up, and says so.
func TestRunRateSkipsWhenTooManyAreOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(400 * time.Millisecond):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	build := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	}
	r := RunRate(context.Background(), srv.Client(), build, 100, 300*time.Millisecond, 5)
	if r.Sent != 5 || r.Skipped < 20 || r.OK != 5 {
		t.Fatalf("want 5 sent and the rest skipped: %+v", r)
	}
}
