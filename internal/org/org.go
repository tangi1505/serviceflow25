// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package org is the organisation tree (Plattform-Kern PL6; Hauptplan E16; architektur.md §5):
// nodes as ltree paths below "root", and settings per node that inherit down the tree. Every
// record carries its node in org_path; rights (PL8) and visibility build on it. A node directly
// below the root may be a tenant (PL44, E136); once there is one, every node there is one.
package org

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
)

// Root is the path of the top node; it exists from the migration on.
const Root = "root"

var (
	// ErrNotFound says there is no such node.
	ErrNotFound = errors.New("org: node not found")
	// ErrExists says the child name is taken below that parent.
	ErrExists = errors.New("org: node exists")
	// ErrNotEmpty says the node still has children.
	ErrNotEmpty = errors.New("org: node has children")
	// ErrInvalid says a name, path or key breaks the rules.
	ErrInvalid = errors.New("org: invalid")
)

var (
	nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	keyRe  = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
)

// Node is one organisational unit.
type Node struct {
	Path  string            `json:"path"`
	Name  string            `json:"name"`
	Label map[string]string `json:"label"`
	// Parent is the path above; empty for the root.
	Parent string `json:"parent,omitempty"`
	// Tenant marks a tenant: directly below the root, with data and accounts of its own.
	Tenant bool `json:"tenant,omitempty"`
}

// Setting is one effective setting of a node with the node it comes from.
type Setting struct {
	Key    string          `json:"key"`
	Value  json.RawMessage `json:"value"`
	Source string          `json:"source"`
}

// Service manages the tree.
type Service struct {
	db *db.Router
}

// New returns a service on the pool.
func New(rt *db.Router) *Service {
	return &Service{db: rt}
}

// tq is the query handle in the database of the request's tenant (PL47).
func (s *Service) tq(ctx context.Context) *queries.Queries { return queries.New(s.db.Tenant(ctx)) }

// at is the query handle in the database of a node (PL47b): the root and the tenants themselves
// are central, a node below a tenant with a database of its own lives there.
func (s *Service) at(path string) *queries.Queries { return queries.New(s.db.For(path)) }

// cq is the query handle in the control database: what belongs to the installation (PL47).
func (s *Service) cq() *queries.Queries { return queries.New(s.db.Control()) }

// ValidPath reports whether a string is a path of this tree: labels of snake_case joined by
// dots, starting at root.
func ValidPath(path string) bool {
	if path == "" {
		return false
	}
	parts := strings.Split(path, ".")
	if parts[0] != Root {
		return false
	}
	for _, p := range parts {
		if !nameRe.MatchString(p) {
			return false
		}
	}
	return true
}

// Exists reports whether a node is there.
func (s *Service) Exists(ctx context.Context, path string) (bool, error) {
	if !ValidPath(path) {
		return false, nil
	}
	ok, err := s.at(path).OrgNodeExists(ctx, path)
	if err != nil {
		return false, fmt.Errorf("org: exists: %w", err)
	}
	return ok, nil
}

// Nodes lists the whole tree in path order.
func (s *Service) Nodes(ctx context.Context) ([]Node, error) {
	rows, err := s.tq(ctx).ListOrgNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("org: list: %w", err)
	}
	out := make([]Node, 0, len(rows))
	for _, r := range rows {
		out = append(out, toNode(r.Path, r.Name, r.Label, r.Tenant))
	}
	return out, nil
}

// Tenants lists the tenants in path order; none means the root is the only one (PL44).
func (s *Service) Tenants(ctx context.Context) ([]Node, error) {
	rows, err := s.cq().ListTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("org: tenants: %w", err)
	}
	out := make([]Node, 0, len(rows))
	for _, r := range rows {
		out = append(out, toNode(r.Path, r.Name, r.Label, true))
	}
	return out, nil
}

// TenantOf is the tenant of the paths a node lies in, the root without one (PL45).
func TenantOf(tenants []string, path string) string {
	for _, t := range tenants {
		if path == t || strings.HasPrefix(path, t+".") {
			return t
		}
	}
	return Root
}

// TenantPaths lists the paths of the tenants; none means the root is the only one.
func (s *Service) TenantPaths(ctx context.Context) ([]string, error) {
	return TenantPaths(ctx, s.cq())
}

// TenantPaths lists the paths of the tenants through the queries of any pool or transaction.
func TenantPaths(ctx context.Context, q *queries.Queries) ([]string, error) {
	rows, err := q.ListTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("org: tenants: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Path)
	}
	return out, nil
}

// Node returns one node.
func (s *Service) Node(ctx context.Context, path string) (Node, error) {
	if !ValidPath(path) {
		return Node{}, ErrNotFound
	}
	r, err := s.at(path).GetOrgNode(ctx, path)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Node{}, ErrNotFound
		}
		return Node{}, fmt.Errorf("org: get: %w", err)
	}
	return toNode(r.Path, r.Name, r.Label, r.Tenant), nil
}

// Create adds a child below parent; the path is parent.name. Directly below the root only
// while there are no tenants: then the nodes there are tenants, made by CreateTenant.
func (s *Service) Create(ctx context.Context, parent, name string, label map[string]string) (Node, error) {
	if parent == Root {
		if tenants, err := s.Tenants(ctx); err != nil {
			return Node{}, err
		} else if len(tenants) > 0 {
			return Node{}, fmt.Errorf("%w: the nodes directly below the root are tenants; create one with itsm tenant create", ErrInvalid)
		}
	}
	return s.insert(ctx, parent, name, label, false)
}

// CreateTenant adds a tenant directly below the root (PL44). The first one needs a root
// without other nodes below: otherwise they would belong to no tenant (itsm tenant create
// --adopt moves them into the first one).
func (s *Service) CreateTenant(ctx context.Context, name string, label map[string]string) (Node, error) {
	nodes, err := s.Nodes(ctx)
	if err != nil {
		return Node{}, err
	}
	for _, n := range nodes {
		if n.Parent == Root && !n.Tenant {
			return Node{}, fmt.Errorf("%w: %s below the root is no tenant; move the tree into the first tenant with --adopt", ErrInvalid, n.Path)
		}
	}
	return s.insert(ctx, Root, name, label, true)
}

func (s *Service) insert(ctx context.Context, parent, name string, label map[string]string, tenant bool) (Node, error) {
	if !nameRe.MatchString(name) {
		return Node{}, fmt.Errorf("%w: name %q must be snake_case (a-z, 0-9, _), at most 63 characters", ErrInvalid, name)
	}
	if ok, err := s.Exists(ctx, parent); err != nil {
		return Node{}, err
	} else if !ok {
		return Node{}, fmt.Errorf("%w: parent %q", ErrNotFound, parent)
	}
	if label == nil {
		label = map[string]string{}
	}
	raw, _ := json.Marshal(label)
	r, err := s.at(parent+"."+name).InsertOrgNode(ctx, queries.InsertOrgNodeParams{Path: parent + "." + name, Name: name, Label: raw, Tenant: tenant})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Node{}, fmt.Errorf("%w: %s.%s", ErrExists, parent, name)
		}
		return Node{}, fmt.Errorf("org: create: %w", err)
	}
	return toNode(r.Path, r.Name, r.Label, r.Tenant), nil
}

// Delete removes a leaf node that is not the root; its settings go with it. Records that
// point at it keep their org_path until PL8 adds the move.
func (s *Service) Delete(ctx context.Context, path string) error {
	if path == Root || !ValidPath(path) {
		return ErrNotFound
	}
	n, err := s.at(path).CountOrgChildren(ctx, path)
	if err != nil {
		return fmt.Errorf("org: children: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w: %s has %d nodes below", ErrNotEmpty, path, n)
	}
	if ok, err := s.Exists(ctx, path); err != nil {
		return err
	} else if !ok {
		return ErrNotFound
	}
	if err := s.at(path).DeleteOrgNode(ctx, path); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return fmt.Errorf("%w: accounts still belong to the tenant %s", ErrNotEmpty, path)
		}
		return fmt.Errorf("org: delete: %w", err)
	}
	return nil
}

// SetSetting stores a value at a node; descendants inherit it unless they set their own.
func (s *Service) SetSetting(ctx context.Context, path, key string, value json.RawMessage, actor *uuid.UUID) error {
	if !keyRe.MatchString(key) {
		return fmt.Errorf("%w: key %q must be dotted snake_case, like ticket.default_priority", ErrInvalid, key)
	}
	if !json.Valid(value) {
		return fmt.Errorf("%w: value is not JSON", ErrInvalid)
	}
	if ok, err := s.Exists(ctx, path); err != nil {
		return err
	} else if !ok {
		return ErrNotFound
	}
	var by uuid.NullUUID
	if actor != nil {
		by = uuid.NullUUID{UUID: *actor, Valid: true}
	}
	if err := s.at(path).UpsertOrgSetting(ctx, queries.UpsertOrgSettingParams{Path: path, Key: key, Value: value, UpdatedBy: by}); err != nil {
		return fmt.Errorf("org: set: %w", err)
	}
	return nil
}

// ApplySettings sets and removes own values of a node in one transaction (SD20: the page
// saves all its changes at once): set holds key to value, unset the keys to remove; a key to
// remove that the node does not set is no error. Checking the values is the caller's.
func (s *Service) ApplySettings(ctx context.Context, path string, set map[string]json.RawMessage, unset []string, actor *uuid.UUID) error {
	for key, value := range set {
		if !keyRe.MatchString(key) {
			return fmt.Errorf("%w: key %q must be dotted snake_case, like ticket.default_priority", ErrInvalid, key)
		}
		if !json.Valid(value) {
			return fmt.Errorf("%w: value of %s is not JSON", ErrInvalid, key)
		}
	}
	if ok, err := s.Exists(ctx, path); err != nil {
		return err
	} else if !ok {
		return ErrNotFound
	}
	var by uuid.NullUUID
	if actor != nil {
		by = uuid.NullUUID{UUID: *actor, Valid: true}
	}
	tx, err := s.db.For(path).Begin(ctx)
	if err != nil {
		return fmt.Errorf("org: apply: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := queries.New(tx)
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if err := q.UpsertOrgSetting(ctx, queries.UpsertOrgSettingParams{Path: path, Key: key, Value: set[key], UpdatedBy: by}); err != nil {
			return fmt.Errorf("org: set %s: %w", key, err)
		}
	}
	for _, key := range unset {
		if _, err := q.DeleteOrgSetting(ctx, queries.DeleteOrgSettingParams{Path: path, Key: key}); err != nil {
			return fmt.Errorf("org: unset %s: %w", key, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("org: apply: %w", err)
	}
	return nil
}

// UnsetSetting removes a node's own value; the node then inherits again.
func (s *Service) UnsetSetting(ctx context.Context, path, key string) error {
	if !ValidPath(path) {
		return ErrNotFound
	}
	n, err := s.at(path).DeleteOrgSetting(ctx, queries.DeleteOrgSettingParams{Path: path, Key: key})
	if err != nil {
		return fmt.Errorf("org: unset: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Settings returns the effective settings of a node: for every key the value of the nearest
// node on the way up, with that node as source.
func (s *Service) Settings(ctx context.Context, path string) ([]Setting, error) {
	if ok, err := s.Exists(ctx, path); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrNotFound
	}
	rows, err := s.at(path).EffectiveOrgSettings(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("org: settings: %w", err)
	}
	if s.db.For(path) != s.db.Control() {
		// The settings of the root are central (PL47b): they count where the tenant sets none.
		set := map[string]bool{}
		for _, r := range rows {
			set[r.Key] = true
		}
		central, err := s.cq().EffectiveOrgSettings(ctx, Root)
		if err != nil {
			return nil, fmt.Errorf("org: settings of the root: %w", err)
		}
		for _, r := range central {
			if !set[r.Key] {
				rows = append(rows, r)
			}
		}
		slices.SortFunc(rows, func(a, b queries.EffectiveOrgSettingsRow) int { return strings.Compare(a.Key, b.Key) })
	}
	out := make([]Setting, 0, len(rows))
	for _, r := range rows {
		out = append(out, Setting{Key: r.Key, Value: r.Value, Source: r.Source})
	}
	return out, nil
}

// Setting returns one effective value, or nil when no node on the way up sets it.
func (s *Service) Setting(ctx context.Context, path, key string) (json.RawMessage, string, error) {
	all, err := s.Settings(ctx, path)
	if err != nil {
		return nil, "", err
	}
	for _, st := range all {
		if st.Key == key {
			return st.Value, st.Source, nil
		}
	}
	return nil, "", nil
}

func toNode(path, name string, label []byte, tenant bool) Node {
	n := Node{Path: path, Name: name, Label: map[string]string{}, Tenant: tenant}
	_ = json.Unmarshal(label, &n.Label)
	if i := strings.LastIndex(path, "."); i > 0 {
		n.Parent = path[:i]
	}
	return n
}
