// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package memo keeps values read from the database in memory without making a request wait
// for a reload (PL58, Kern-Plan E66).
//
// The caches of the core (SLAs, rules, settings, types) reloaded inside a request: the one
// that found the value too old took a lock and read anew. A write holds its transaction, and
// with it a connection, while it asks the cache; the reload needs a second one. With as many
// writes at once as the pool has connections, every connection waited for the lock and the
// reload for a connection, until the requests gave up (found with H14: every create of an
// incident failed with 32 requests at once and 16 connections).
//
// A Value hands out the last state at once. A state older than MaxAge, or marked stale, is
// read anew in the background, one load at a time; until it is there the old one serves.
// Only the very first load, when there is no state yet, runs in the caller; Refresh reloads at
// once for a writer after its commit, so that its own change counts for its next request.
package memo

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// backgroundTimeout bounds a load in the background.
const backgroundTimeout = 30 * time.Second

// Value is a value loaded from the database, kept in memory.
type Value[T any] struct {
	// MaxAge is how long a state counts as fresh; zero means until marked stale.
	MaxAge time.Duration
	// Load reads the value. It runs with the context of the request that triggered it, detached
	// from its end: the values in it (rights of the system, the database bound) stay.
	Load func(context.Context) (T, error)
	// Logger reports a failed background load; nil uses slog.Default.
	Logger *slog.Logger
	// Name names the value in the log.
	Name string

	mu      sync.Mutex
	val     T
	ok      bool
	at      time.Time
	stale   bool
	loading bool
	// gen counts the states; a background load started before a Refresh does not overwrite it.
	gen uint64
}

// Get returns the value: the last state at once, or the first load in the caller.
func (v *Value[T]) Get(ctx context.Context) (T, error) {
	v.mu.Lock()
	if !v.ok {
		v.mu.Unlock()
		return v.Refresh(ctx)
	}
	val := v.val
	old := v.stale || (v.MaxAge > 0 && time.Since(v.at) > v.MaxAge)
	if old && !v.loading {
		v.loading = true
		gen := v.gen
		go v.background(context.WithoutCancel(ctx), gen)
	}
	v.mu.Unlock()
	return val, nil
}

func (v *Value[T]) background(ctx context.Context, gen uint64) {
	ctx, cancel := context.WithTimeout(ctx, backgroundTimeout)
	defer cancel()
	val, err := v.Load(ctx)
	v.mu.Lock()
	defer v.mu.Unlock()
	v.loading = false
	if err != nil {
		logger := v.Logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn("memo: reload failed, keeping the last state", "value", v.Name, "error", err.Error())
		return
	}
	if v.gen != gen {
		return // a Refresh came in between with a newer state
	}
	v.set(val)
}

// Refresh loads the value now, in the caller, and keeps it.
func (v *Value[T]) Refresh(ctx context.Context) (T, error) {
	val, err := v.Load(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	v.mu.Lock()
	v.set(val)
	v.mu.Unlock()
	return val, nil
}

func (v *Value[T]) set(val T) {
	v.val, v.ok, v.at, v.stale = val, true, time.Now(), false
	v.gen++
}

// Stale marks the state as old: the next Get reloads it in the background.
func (v *Value[T]) Stale() {
	v.mu.Lock()
	v.stale = true
	v.mu.Unlock()
}

// Forget drops the state: the next Get loads in the caller.
func (v *Value[T]) Forget() {
	v.mu.Lock()
	var zero T
	v.val, v.ok, v.stale = zero, false, false
	v.gen++
	v.mu.Unlock()
}
