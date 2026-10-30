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

func TestPercentile(t *testing.T) {
	var v []time.Duration
	for i := 1; i <= 100; i++ {
		v = append(v, time.Duration(i)*time.Millisecond)
	}
	cases := map[float64]time.Duration{50: 50 * time.Millisecond, 95: 95 * time.Millisecond, 99: 99 * time.Millisecond, 100: 100 * time.Millisecond}
	for p, want := range cases {
		if got := Percentile(v, p); got != want {
			t.Errorf("p%v = %v, want %v", p, got, want)
		}
	}
	if Percentile(nil, 95) != 0 {
		t.Error("empty input must give 0")
	}
}

func TestRunCountsErrorsAndLatency(t *testing.T) {
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1)%10 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		time.Sleep(2 * time.Millisecond)
	}))
	defer srv.Close()

	req := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	}
	r := Run(t.Context(), srv.Client(), req, Options{Workers: 4, Warmup: 50 * time.Millisecond, Duration: 300 * time.Millisecond})
	if r.Requests == 0 || r.Errors == 0 {
		t.Fatalf("expected requests and errors, got %+v", r)
	}
	if r.P95ms < 2 {
		t.Fatalf("p95 %.3f ms below the server delay", r.P95ms)
	}
	if r.RPS <= 0 {
		t.Fatalf("rps = %v", r.RPS)
	}
}

func TestBest(t *testing.T) {
	got := Best([]Result{{P95ms: 9}, {P95ms: 1}, {P95ms: 5}})
	if got.P95ms != 1 {
		t.Fatalf("best p95 = %v, want 1", got.P95ms)
	}
	if Best(nil).P95ms != 0 {
		t.Fatal("best of nothing must be empty")
	}
}
