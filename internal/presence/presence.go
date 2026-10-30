// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package presence keeps who has a record open (UI8; Hauptplan E130, E29): a page beats every
// 20 seconds and says whether its form holds unsaved changes; whoever has not beaten for 60
// seconds is gone. The beats live in Valkey, one hash per record, so that every instance of
// the API sees the same; nothing of it reaches PostgreSQL.
package presence

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/cache"
)

// TTL is how long a beat counts.
const TTL = 60 * time.Second

// Entry is one person on a record.
type Entry struct {
	Account uuid.UUID `json:"account"`
	Name    string    `json:"name"`
	Editing bool      `json:"editing"`
	Since   time.Time `json:"since"`
	At      time.Time `json:"at"`
}

// Service keeps the beats.
type Service struct {
	kv  *cache.Client
	Now func() time.Time
}

// New returns the service.
func New(kv *cache.Client) *Service { return &Service{kv: kv, Now: time.Now} }

func key(typeName string, id uuid.UUID) string { return "presence:" + typeName + ":" + id.String() }

// Beat says that the account has the record open (editing: with unsaved changes) and returns
// the others on it, those editing first, then by name.
func (s *Service) Beat(ctx context.Context, typeName string, id, account uuid.UUID, name string, editing bool) ([]Entry, error) {
	k := key(typeName, id)
	all, err := s.read(ctx, k)
	if err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	me := Entry{Account: account, Name: name, Editing: editing, Since: now, At: now}
	if old, ok := all[account]; ok {
		me.Since = old.Since
	}
	raw, _ := json.Marshal(me)
	// The hash lives twice as long as a beat, so that a stale field is seen and removed.
	if err := s.kv.HSetExpire(ctx, k, account.String(), raw, 2*TTL); err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(all))
	for acc, e := range all {
		if acc != account {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b Entry) int {
		if a.Editing != b.Editing {
			if a.Editing {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// Leave takes the account off the record.
func (s *Service) Leave(ctx context.Context, typeName string, id, account uuid.UUID) error {
	return s.kv.HDel(ctx, key(typeName, id), account.String())
}

// read returns the live beats of a record and removes the stale ones.
func (s *Service) read(ctx context.Context, k string) (map[uuid.UUID]Entry, error) {
	m, err := s.kv.HGetAll(ctx, k)
	if err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	out := make(map[uuid.UUID]Entry, len(m))
	var stale []string
	for field, raw := range m {
		var e Entry
		if json.Unmarshal([]byte(raw), &e) != nil || now.Sub(e.At) > TTL {
			stale = append(stale, field)
			continue
		}
		out[e.Account] = e
	}
	if err := s.kv.HDel(ctx, k, stale...); err != nil {
		return nil, err
	}
	return out, nil
}
