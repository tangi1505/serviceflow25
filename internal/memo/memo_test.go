// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package memo

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A stale value is handed out at once while one load runs in the background; nobody waits
// for it (PL58: the pool of a server ran dry while every write waited for the reload).
func TestStaleServesWhileOneLoadRuns(t *testing.T) {
	var loads atomic.Int32
	release := make(chan struct{})
	v := &Value[int]{MaxAge: time.Hour, Name: "test"}
	v.Load = func(context.Context) (int, error) {
		n := loads.Add(1)
		if n > 1 {
			<-release
		}
		return int(n), nil
	}
	ctx := context.Background()
	if got, err := v.Get(ctx); err != nil || got != 1 {
		t.Fatalf("first load in the caller: %d %v", got, err)
	}
	v.Stale()
	// Many callers at once: all get the old state without waiting, one load starts.
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			done := make(chan int, 1)
			go func() {
				got, _ := v.Get(ctx)
				done <- got
			}()
			select {
			case got := <-done:
				if got != 1 {
					t.Errorf("got %d, want the old state 1", got)
				}
			case <-time.After(2 * time.Second):
				t.Error("a caller waited for the reload")
			}
		})
	}
	wg.Wait()
	if n := loads.Load(); n != 2 {
		t.Fatalf("%d loads, want the first and one in the background", n)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got, _ := v.Get(ctx); got == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background load did not arrive")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Refresh loads at once for the writer; a background load that started before it does not
// overwrite its newer state; a failed background load keeps the last state; Forget makes the
// next caller load.
func TestRefreshForgetAndFailedLoads(t *testing.T) {
	var next atomic.Int32
	fail := atomic.Bool{}
	slow := make(chan struct{})
	started := make(chan struct{})
	v := &Value[int]{Name: "test"}
	v.Load = func(context.Context) (int, error) {
		if fail.Load() {
			return 0, errors.New("database away")
		}
		n := next.Add(1)
		if n == 2 {
			close(started)
			<-slow // the background load, overtaken by the Refresh below
		}
		return int(n), nil
	}
	ctx := context.Background()
	if got, _ := v.Get(ctx); got != 1 {
		t.Fatalf("first: %d", got)
	}
	v.Stale()
	_, _ = v.Get(ctx) // starts the slow background load (2)
	<-started
	if got, err := v.Refresh(ctx); err != nil || got != 3 {
		t.Fatalf("refresh: %d %v", got, err)
	}
	close(slow)
	time.Sleep(50 * time.Millisecond)
	if got, _ := v.Get(ctx); got != 3 {
		t.Fatalf("the older background load overwrote the refresh: %d", got)
	}
	fail.Store(true)
	v.Stale()
	_, _ = v.Get(ctx)
	time.Sleep(50 * time.Millisecond)
	if got, err := v.Get(ctx); err != nil || got != 3 {
		t.Fatalf("a failed reload must keep the last state: %d %v", got, err)
	}
	v.Forget()
	if _, err := v.Get(ctx); err == nil {
		t.Fatal("after Forget the caller loads and sees the error")
	}
	fail.Store(false)
	if got, err := v.Get(ctx); err != nil || got != 4 {
		t.Fatalf("after Forget: %d %v", got, err)
	}
}
