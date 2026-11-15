// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"group24.de/itsm/internal/meta"
)

// --- the types in memory (PL53, found with H14) ---
//
// The definitions were read from the control database and decoded for every call of Type;
// with a write of an incident that was more than half of the time. Now they are kept in
// memory and loaded anew when the definitions changed: at once after a write of this process
// (meta.DefinitionsChanged), on the notice of the channel itsm_definitions for the writes of
// other instances and of the command line (WatchTypes), and at the latest after typesMaxAge
// in case a notice got lost. The types handed out are shared: nobody may change them.

// typesMaxAge is the longest the types stay in memory without a notice.
const typesMaxAge = time.Minute

type typeCache struct {
	mu      sync.Mutex
	gen     int64
	at      time.Time
	byID    map[string]meta.Type
	stale   bool
	loading bool
}

// get returns the types. A write of this process (a new generation) loads them in the caller,
// so that the change counts for its next request; the minute run out or a notice of another
// instance reads them anew in the background while the last state serves (PL58: a reload in a
// request that holds a transaction waits for a second connection of the pool).
func (c *typeCache) get(ctx context.Context, load func(context.Context) (map[string]meta.Type, error)) (map[string]meta.Type, error) {
	c.mu.Lock()
	gen := meta.DefinitionsGeneration()
	if c.byID != nil && c.gen == gen {
		if (c.stale || time.Since(c.at) >= typesMaxAge) && !c.loading {
			c.loading = true
			go c.reload(context.WithoutCancel(ctx), gen, load)
		}
		m := c.byID
		c.mu.Unlock()
		return m, nil
	}
	defer c.mu.Unlock()
	m, err := load(ctx)
	if err != nil {
		return nil, err
	}
	c.byID, c.gen, c.at, c.stale = m, gen, time.Now(), false
	return m, nil
}

func (c *typeCache) reload(ctx context.Context, gen int64, load func(context.Context) (map[string]meta.Type, error)) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	m, err := load(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loading = false
	if err != nil || c.gen != gen {
		return // the last state stays; a write of this process loaded a newer one meanwhile
	}
	c.byID, c.at, c.stale = m, time.Now(), false
}

// markStale makes the next call read the types anew in the background.
func (c *typeCache) markStale() {
	c.mu.Lock()
	c.stale = true
	c.mu.Unlock()
}

// definitionsChannel is the channel the trigger of the table definition notifies on.
const definitionsChannel = "itsm_definitions"

// WatchTypes listens on the channel itsm_definitions of the control database and tells the
// types in memory of every change. It blocks until the context ends.
func (s *Service) WatchTypes(ctx context.Context, logger *slog.Logger) {
	for ctx.Err() == nil {
		err := s.listenTypes(ctx)
		if ctx.Err() != nil {
			return
		}
		// A notice may have got lost while the connection was away.
		s.typeCache.markStale()
		logger.Warn("records: listening for changes of the definitions failed, retrying", "error", err.Error())
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func (s *Service) listenTypes(ctx context.Context) error {
	conn, err := s.db.Control().Acquire(ctx)
	if err != nil {
		return err
	}
	// The connection listens; it does not go back into the pool as it is.
	defer func() {
		_ = conn.Conn().Close(context.Background())
		conn.Release()
	}()
	if _, err := conn.Exec(ctx, "LISTEN "+definitionsChannel); err != nil {
		return err
	}
	// Changes before the LISTEN are not noticed.
	s.typeCache.markStale()
	for {
		if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
			return err
		}
		// A change of another instance or the command line: read anew in the background (PL58);
		// a write of this process has loaded already.
		s.typeCache.markStale()
	}
}
