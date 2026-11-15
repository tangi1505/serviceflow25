// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package settings knows the settings of the organisation tree the packages declare (SD20;
// Hauptplan E131, E180): which keys there are, of what kind, with which default. It checks
// values before the organisation tree stores them, writes the changes of a node at once, and
// gives the value of a setting at a node with the default of its declaration (formulas,
// SLAs).
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/memo"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
)

// SourceDefault is the source of a value that comes from the declaration.
const SourceDefault = "default"

// reloadAfter is how long the declarations stay before they are read again: a package
// installed by the command reaches a running server within this time.
const reloadAfter = 30 * time.Second

// valueTTL is how long a value read at a node stays; a change through this instance clears
// them at once, the other instances see it within this time.
const valueTTL = 10 * time.Second

type cached struct {
	raw     json.RawMessage
	source  string
	at      time.Time
	loading bool
}

// declared are the declarations of the installed packages.
type declared struct {
	defs  []meta.Setting
	byKey map[string]meta.Setting
}

// Service keeps the declarations and reads values.
type Service struct {
	db  *db.Router
	org *org.Service
	// Now is the clock; tests replace it.
	Now func() time.Time

	// decl are the declarations in memory (PL58): a request takes the last state, a reload runs
	// in the background, never in a request that holds a transaction.
	decl memo.Value[declared]
	mu   sync.Mutex
	// values are the values read at nodes; an old one serves while it is read anew in the
	// background (PL58: the priority of every incident reads one in its write).
	values map[string]*cached
}

// New returns the service.
func New(rt *db.Router, o *org.Service) *Service {
	s := &Service{db: rt, org: o, Now: time.Now, values: map[string]*cached{}}
	s.decl = memo.Value[declared]{MaxAge: reloadAfter, Load: s.readDeclared, Name: "settings"}
	return s
}

// Warm loads the declarations before the first request (PL58).
func (s *Service) Warm(ctx context.Context) error {
	_, err := s.decl.Refresh(ctx)
	return err
}

// Definitions are the declared settings of the installed packages: the core first, then the
// packages by name, each in the order of its package.yaml.
func (s *Service) Definitions(ctx context.Context) ([]meta.Setting, error) {
	d, err := s.decl.Get(ctx)
	if err != nil {
		return nil, err
	}
	return d.defs, nil
}

// readDeclared reads the declarations from the packages.
func (s *Service) readDeclared(ctx context.Context) (declared, error) {
	rows, err := queries.New(s.db.Control()).ListPackages(ctx)
	if err != nil {
		return declared{}, fmt.Errorf("settings: packages: %w", err)
	}
	slices.SortStableFunc(rows, func(a, b queries.Package) int {
		switch {
		case a.Name == b.Name:
			return 0
		case a.Name == "core":
			return -1
		case b.Name == "core":
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	var defs []meta.Setting
	byKey := map[string]meta.Setting{}
	for _, r := range rows {
		var p meta.Package
		if err := json.Unmarshal(r.Spec, &p); err != nil {
			return declared{}, fmt.Errorf("settings: package %s: %w", r.Name, err)
		}
		for _, st := range p.Settings {
			if _, twice := byKey[st.Key]; twice {
				continue // the first package keeps a key (meta.Store refuses a second)
			}
			st.Package = r.Name
			defs = append(defs, st)
			byKey[st.Key] = st
		}
	}
	return declared{defs: defs, byKey: byKey}, nil
}

// Definition is the declaration of a key. A key not known reads the declarations again: a
// package installed by the command declares it at once.
func (s *Service) Definition(ctx context.Context, key string) (meta.Setting, bool, error) {
	d, err := s.decl.Get(ctx)
	if err != nil {
		return meta.Setting{}, false, err
	}
	if st, ok := d.byKey[key]; ok {
		return st, true, nil
	}
	if d, err = s.decl.Refresh(ctx); err != nil {
		return meta.Setting{}, false, err
	}
	st, ok := d.byKey[key]
	return st, ok, nil
}

// Forget drops what the service holds: the next read asks the database.
func (s *Service) Forget() {
	s.decl.Forget()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = map[string]*cached{}
}

// Value is the setting at the node: its own or the nearest above, else the default of the
// declaration (source "default"), else nil.
func (s *Service) Value(ctx context.Context, path, key string) (json.RawMessage, string, error) {
	ck := path + "\x00" + key
	s.mu.Lock()
	if c, ok := s.values[ck]; ok {
		if s.Now().Sub(c.at) >= valueTTL && !c.loading {
			c.loading = true
			go s.reloadValue(context.WithoutCancel(ctx), ck, path, key)
		}
		raw, src := c.raw, c.source
		s.mu.Unlock()
		return raw, src, nil
	}
	s.mu.Unlock()
	raw, src, err := s.read(ctx, path, key)
	if err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	s.values[ck] = &cached{raw: raw, source: src, at: s.Now()}
	s.mu.Unlock()
	return raw, src, nil
}

// read reads the value at the node, else the default of the declaration.
func (s *Service) read(ctx context.Context, path, key string) (json.RawMessage, string, error) {
	raw, src, err := s.org.Setting(ctx, path, key)
	if err != nil {
		return nil, "", err
	}
	if raw == nil {
		st, ok, err := s.Definition(ctx, key)
		if err != nil {
			return nil, "", err
		}
		if ok {
			raw, src = st.DefaultJSON(), SourceDefault
		}
	}
	return raw, src, nil
}

// reloadValue reads a value anew in the background; a failed read keeps the old one.
func (s *Service) reloadValue(ctx context.Context, ck, path, key string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, src, err := s.read(ctx, path, key)
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.values[ck]
	if !ok {
		return // cleared by a change in between; the next read loads afresh
	}
	c.loading = false
	if err == nil {
		c.raw, c.source, c.at = raw, src, s.Now()
	}
}

// Check says why value cannot be the setting key at the node: a key no package declares, a
// key of the root elsewhere, a value its declaration refuses. nil when it can.
func (s *Service) Check(ctx context.Context, path, key string, value json.RawMessage) (*records.Problem, error) {
	st, ok, err := s.Definition(ctx, key)
	if err != nil {
		return nil, err
	}
	switch {
	case !ok:
		return &records.Problem{Field: key, Code: "unknown", Message: "no package declares this setting"}, nil
	case st.Root && path != org.Root:
		return &records.Problem{Field: key, Code: "root", Message: "can only be set at the root node"}, nil
	}
	// A value that does not pass is a problem of the input, not an error of the service.
	if bad := st.Check(value); bad != nil {
		return &records.Problem{Field: key, Code: "invalid", Message: bad.Error()}, nil //nolint:nilerr // see above
	}
	return nil, nil
}

// Set checks and stores one value (PUT …/settings/{key}).
func (s *Service) Set(ctx context.Context, path, key string, value json.RawMessage, actor records.Actor) error {
	return s.Apply(ctx, path, map[string]json.RawMessage{key: value}, actor)
}

// Apply checks every change and writes them all or none (SD20: the page saves at once). A
// change of null removes the node's own value; that works for any key, also one no package
// declares any more.
func (s *Service) Apply(ctx context.Context, path string, changes map[string]json.RawMessage, actor records.Actor) error {
	set := map[string]json.RawMessage{}
	var unset []string
	var ps []records.Problem
	keys := make([]string, 0, len(changes))
	for k := range changes {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, key := range keys {
		value := changes[key]
		if len(value) == 0 || string(value) == "null" {
			unset = append(unset, key)
			continue
		}
		p, err := s.Check(ctx, path, key, value)
		if err != nil {
			return err
		}
		if p != nil {
			ps = append(ps, *p)
			continue
		}
		set[key] = value
	}
	if len(ps) > 0 {
		return &records.ValidationError{Problems: ps}
	}
	if err := s.org.ApplySettings(ctx, path, set, unset, (*uuid.UUID)(actor)); err != nil {
		return err
	}
	s.mu.Lock()
	s.values = map[string]*cached{}
	s.mu.Unlock()
	return nil
}
