// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"group24.de/itsm/internal/records"
)

// An evaluation waits for a free place in the budget (PL67), at most as long as its request
// may take; then it ends without having touched the database.
func TestBudgetWaitsForAPlace(t *testing.T) {
	s := New(nil)
	s.Limit(1, time.Second)
	s.budget <- struct{}{} // the one place is taken
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	t0 := time.Now()
	_, _, err := s.query(ctx, "probe.probe", records.Query{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
	if d := time.Since(t0); d < 40*time.Millisecond {
		t.Fatalf("returned after %s without waiting", d)
	}
	if New(nil).budget != nil {
		t.Fatal("a service without Limit has a budget")
	}
	s0 := New(nil)
	s0.Limit(0, 0)
	if s0.budget != nil {
		t.Fatal("Limit(0) is no bound")
	}
}
