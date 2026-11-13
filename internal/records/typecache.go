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
	mu   sync.Mutex
	gen  int64
	at   time.Time
	byID map[string]meta.Type
}

func (c *typeCache) get(ctx context.Context, load func(context.Context) (map[string]meta.Type, error)) (map[string]meta.Type, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	gen := meta.DefinitionsGeneration()
	if c.byID != nil && c.gen == gen && time.Since(c.at) < typesMaxAge {
		return c.byID, nil
	}
	m, err := load(ctx)
	if err != nil {
		return nil, err
	}
	c.byID, c.gen, c.at = m, gen, time.Now()
	return m, nil
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
		meta.DefinitionsChanged()
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
	meta.DefinitionsChanged()
	for {
		if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
			return err
		}
		meta.DefinitionsChanged()
	}
}
