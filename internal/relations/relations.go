// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package relations links records of any types (Hauptplan KF2, E92; PL15): kinds come from
// the packages (relations/*.yaml), the links live in the core table relation, graph queries
// walk them with a depth limit, and the impact analysis follows the kinds marked impact
// against their direction. Relating leaves audit and event at both ends.
package relations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// MaxDepth bounds a graph walk; DefaultDepth is used when the caller names none.
const (
	MaxDepth     = 10
	DefaultDepth = 5
)

var (
	// ErrNotFound says there is no such relation or kind.
	ErrNotFound = errors.New("relations: not found")
	// ErrInvalid says the request cannot be fulfilled as given (422).
	ErrInvalid = errors.New("relations: invalid")
)

// End is one side of a relation.
type End struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

func (e End) String() string { return e.Type + "/" + e.ID.String() }

// Relation is a link as the API shows it.
type Relation struct {
	ID        uuid.UUID  `json:"id"`
	Kind      string     `json:"kind"`
	From      End        `json:"from"`
	To        End        `json:"to"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
}

// Node is a record reached by a walk, with its depth and its title.
type Node struct {
	Type  string    `json:"type"`
	ID    uuid.UUID `json:"id"`
	Depth int       `json:"depth"`
	Title any       `json:"title,omitempty"`
}

// Edge is a relation of a walk with the depth it was reached at.
type Edge struct {
	ID    uuid.UUID `json:"id"`
	Kind  string    `json:"kind"`
	From  End       `json:"from"`
	To    End       `json:"to"`
	Depth int       `json:"depth"`
}

// Graph is the answer of a walk: the start is the node at depth 0.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Walk configures a graph query.
type Walk struct {
	Kinds     []string
	Direction string // out, in or both
	Depth     int
}

// Service is the relation store.
type Service struct {
	db  *db.Router
	rec *records.Service
}

// New wires the service.
func New(rt *db.Router, rec *records.Service) *Service {
	return &Service{db: rt, rec: rec}
}

// Kinds lists the installed relation kinds in name order.
func (s *Service) Kinds(ctx context.Context) ([]meta.RelationKind, error) {
	rows, err := queries.New(s.db.Control()).ListDefinitions(ctx, meta.KindRelation)
	if err != nil {
		return nil, fmt.Errorf("relations: kinds: %w", err)
	}
	out := make([]meta.RelationKind, 0, len(rows))
	for _, r := range rows {
		var k meta.RelationKind
		if err := json.Unmarshal(r.Spec, &k); err != nil {
			return nil, fmt.Errorf("relations: kind %s: %w", r.Name, err)
		}
		k.Package = r.Package
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].QualifiedName() < out[j].QualifiedName() })
	return out, nil
}

// Kind returns one kind by its qualified name.
func (s *Service) Kind(ctx context.Context, name string) (meta.RelationKind, error) {
	kinds, err := s.Kinds(ctx)
	if err != nil {
		return meta.RelationKind{}, err
	}
	for _, k := range kinds {
		if k.QualifiedName() == name {
			return k, nil
		}
	}
	return meta.RelationKind{}, fmt.Errorf("%w: kind %s", ErrNotFound, name)
}

func toRelation(r queries.Relation) Relation {
	out := Relation{ID: r.ID, Kind: r.Kind, From: End{r.FromType, r.FromID}, To: End{r.ToType, r.ToID}, CreatedAt: r.CreatedAt.Time}
	if r.CreatedBy.Valid {
		by := r.CreatedBy.UUID
		out.CreatedBy = &by
	}
	return out
}

// Create links two records. The kind must allow the types, and the session must be allowed
// to update both records (a relation changes both). Both ends get an audit row and an event.
func (s *Service) Create(ctx context.Context, kind string, from, to End, actor records.Actor) (Relation, error) {
	k, err := s.Kind(ctx, kind)
	if err != nil {
		return Relation{}, fmt.Errorf("%w: unknown kind %s", ErrInvalid, kind)
	}
	if !k.Allows(from.Type, to.Type) {
		return Relation{}, fmt.Errorf("%w: %s does not link %s to %s", ErrInvalid, kind, from.Type, to.Type)
	}
	if from == to {
		return Relation{}, fmt.Errorf("%w: a record cannot relate to itself", ErrInvalid)
	}
	for _, e := range []End{from, to} {
		if _, err := s.rec.Check(ctx, e.Type, e.ID, authz.ActionUpdate); err != nil {
			return Relation{}, err
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Relation{}, err
	}
	var by uuid.NullUUID
	if actor != nil {
		by = uuid.NullUUID{UUID: *actor, Valid: true}
	}
	row, err := queries.New(s.db.Tenant(ctx)).InsertRelation(ctx, queries.InsertRelationParams{
		ID: id, Kind: kind, FromType: from.Type, FromID: from.ID, ToType: to.Type, ToID: to.ID, CreatedBy: by,
	})
	if err != nil {
		return Relation{}, fmt.Errorf("relations: create: %w", err)
	}
	rel := toRelation(row)
	if err := s.trace(ctx, rel, "relate", actor); err != nil {
		return Relation{}, err
	}
	return rel, nil
}

// trace writes audit and event at both ends of a relation.
func (s *Service) trace(ctx context.Context, rel Relation, action string, actor records.Actor) error {
	for _, side := range []struct {
		own, other End
		dir        string
	}{{rel.From, rel.To, "to"}, {rel.To, rel.From, "from"}} {
		payload := map[string]any{"relation": rel.ID, "kind": rel.Kind, side.dir: side.other}
		if err := s.rec.Trace(ctx, side.own.Type, side.own.ID, action, payload, actor); err != nil {
			return err
		}
	}
	return nil
}

// Get returns one relation when the session may read both ends.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Relation, error) {
	row, err := queries.New(s.db.Tenant(ctx)).GetRelation(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Relation{}, fmt.Errorf("%w: relation %s", ErrNotFound, id)
	}
	if err != nil {
		return Relation{}, fmt.Errorf("relations: get: %w", err)
	}
	rel := toRelation(row)
	for _, e := range []End{rel.From, rel.To} {
		if _, err := s.rec.Check(ctx, e.Type, e.ID, authz.ActionRead); err != nil {
			return Relation{}, fmt.Errorf("%w: relation %s", ErrNotFound, id)
		}
	}
	return rel, nil
}

// Delete removes a relation; the session must be allowed to update both ends.
func (s *Service) Delete(ctx context.Context, id uuid.UUID, actor records.Actor) error {
	rel, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	for _, e := range []End{rel.From, rel.To} {
		if _, err := s.rec.Check(ctx, e.Type, e.ID, authz.ActionUpdate); err != nil {
			return err
		}
	}
	if _, err := queries.New(s.db.Tenant(ctx)).DeleteRelation(ctx, id); err != nil {
		return fmt.Errorf("relations: delete: %w", err)
	}
	return s.trace(ctx, rel, "unrelate", actor)
}

// Of lists the relations of a record (direction out, in or both; kind optional), only those
// whose other end the session may read.
func (s *Service) Of(ctx context.Context, rec End, kind, direction string) ([]Relation, error) {
	if _, err := s.rec.Check(ctx, rec.Type, rec.ID, authz.ActionRead); err != nil {
		return nil, err
	}
	out, in, err := directions(direction)
	if err != nil {
		return nil, err
	}
	var k *string
	if kind != "" {
		k = &kind
	}
	rows, err := queries.New(s.db.Tenant(ctx)).ListRelationsOf(ctx, queries.ListRelationsOfParams{TypeName: rec.Type, RecordID: rec.ID, Outgoing: out, Incoming: in, Kind: textOrNull(k)})
	if err != nil {
		return nil, fmt.Errorf("relations: of %s: %w", rec, err)
	}
	visible := newVisibility(s.rec)
	list := make([]Relation, 0, len(rows))
	for _, r := range rows {
		rel := toRelation(r)
		other := rel.To
		if other == rec {
			other = rel.From
		}
		if visible.readable(ctx, other) {
			list = append(list, rel)
		}
	}
	return list, nil
}

func directions(d string) (out, in bool, err error) {
	switch d {
	case "", "both":
		return true, true, nil
	case "out":
		return true, false, nil
	case "in":
		return false, true, nil
	}
	return false, false, fmt.Errorf("%w: direction must be out, in or both", ErrInvalid)
}

// Graph walks the relations from a record up to the depth (recursive CTE with a path
// against cycles), then drops what the session may not read; the start is depth 0.
func (s *Service) Graph(ctx context.Context, start End, w Walk) (Graph, error) {
	if _, err := s.rec.Check(ctx, start.Type, start.ID, authz.ActionRead); err != nil {
		return Graph{}, err
	}
	out, in, err := directions(w.Direction)
	if err != nil {
		return Graph{}, err
	}
	depth := w.Depth
	if depth <= 0 {
		depth = DefaultDepth
	}
	if depth > MaxDepth {
		return Graph{}, fmt.Errorf("%w: depth at most %d", ErrInvalid, MaxDepth)
	}
	var kinds []string
	for _, k := range w.Kinds {
		if k = strings.TrimSpace(k); k != "" {
			if _, err := s.Kind(ctx, k); err != nil {
				return Graph{}, fmt.Errorf("%w: unknown kind %s", ErrInvalid, k)
			}
			kinds = append(kinds, k)
		}
	}
	edges := map[uuid.UUID]Edge{}
	if out {
		if err := s.walk(ctx, start, kinds, depth, true, edges); err != nil {
			return Graph{}, err
		}
	}
	if in {
		if err := s.walk(ctx, start, kinds, depth, false, edges); err != nil {
			return Graph{}, err
		}
	}
	return s.assemble(ctx, start, edges), nil
}

// walk runs the recursive CTE in one direction and merges the edges (lowest depth wins).
func (s *Service) walk(ctx context.Context, start End, kinds []string, depth int, outgoing bool, edges map[uuid.UUID]Edge) error {
	near, far := "from", "to"
	if !outgoing {
		near, far = "to", "from"
	}
	// An edge back into the path (a cycle) is returned but not walked on from (cont false).
	sql := fmt.Sprintf(`WITH RECURSIVE walk AS (
    SELECT r.id, r.kind, r.from_type, r.from_id, r.to_type, r.to_id, 1 AS depth, ARRAY[r.%[1]s_id, r.%[2]s_id] AS path, true AS cont
    FROM relation r
    WHERE r.%[1]s_type = $1 AND r.%[1]s_id = $2 AND ($3::text[] IS NULL OR r.kind = ANY($3))
  UNION ALL
    SELECT r.id, r.kind, r.from_type, r.from_id, r.to_type, r.to_id, w.depth + 1, w.path || r.%[2]s_id, NOT (r.%[2]s_id = ANY(w.path))
    FROM relation r JOIN walk w ON r.%[1]s_type = w.%[2]s_type AND r.%[1]s_id = w.%[2]s_id
    WHERE w.cont AND w.depth < $4 AND ($3::text[] IS NULL OR r.kind = ANY($3))
)
SELECT DISTINCT ON (id) id, kind, from_type, from_id, to_type, to_id, depth FROM walk ORDER BY id, depth`, near, far)
	var kindArg any
	if len(kinds) > 0 {
		kindArg = kinds
	}
	rows, err := s.db.Tenant(ctx).Query(ctx, sql, start.Type, start.ID, kindArg, depth)
	if err != nil {
		return fmt.Errorf("relations: walk: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.ID, &e.Kind, &e.From.Type, &e.From.ID, &e.To.Type, &e.To.ID, &e.Depth); err != nil {
			return err
		}
		if have, ok := edges[e.ID]; !ok || e.Depth < have.Depth {
			edges[e.ID] = e
		}
	}
	return rows.Err()
}

// assemble turns edges into a graph: nodes at their lowest depth, unreadable records and
// the edges that touch them left out, everything in a stable order.
func (s *Service) assemble(ctx context.Context, start End, edges map[uuid.UUID]Edge) Graph {
	visible := newVisibility(s.rec)
	visible.seen[start] = true
	nodeDepth := map[End]int{start: 0}
	list := make([]Edge, 0, len(edges))
	for _, e := range edges {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Depth != list[j].Depth {
			return list[i].Depth < list[j].Depth
		}
		return list[i].ID.String() < list[j].ID.String()
	})
	g := Graph{Edges: []Edge{}}
	for _, e := range list {
		if !visible.readable(ctx, e.From) || !visible.readable(ctx, e.To) {
			continue
		}
		for _, end := range []End{e.From, e.To} {
			if d, ok := nodeDepth[end]; !ok || e.Depth < d {
				if _, ok := nodeDepth[end]; !ok {
					nodeDepth[end] = e.Depth
				}
			}
		}
		g.Edges = append(g.Edges, e)
	}
	for end, d := range nodeDepth {
		g.Nodes = append(g.Nodes, Node{Type: end.Type, ID: end.ID, Depth: d, Title: visible.title(ctx, end)})
	}
	sort.Slice(g.Nodes, func(i, j int) bool {
		if g.Nodes[i].Depth != g.Nodes[j].Depth {
			return g.Nodes[i].Depth < g.Nodes[j].Depth
		}
		return g.Nodes[i].ID.String() < g.Nodes[j].ID.String()
	})
	return g
}

// Impact lists what depends on a record: the walk against the direction of every kind marked
// impact (E92), up to the depth.
func (s *Service) Impact(ctx context.Context, rec End, depth int) (Graph, error) {
	kinds, err := s.Kinds(ctx)
	if err != nil {
		return Graph{}, err
	}
	var names []string
	for _, k := range kinds {
		if k.Impact {
			names = append(names, k.QualifiedName())
		}
	}
	if len(names) == 0 {
		if _, err := s.rec.Check(ctx, rec.Type, rec.ID, authz.ActionRead); err != nil {
			return Graph{}, err
		}
		return Graph{Nodes: []Node{{Type: rec.Type, ID: rec.ID, Depth: 0}}, Edges: []Edge{}}, nil
	}
	return s.Graph(ctx, rec, Walk{Kinds: names, Direction: "in", Depth: depth})
}

// visibility remembers which records the session may read, and their titles.
type visibility struct {
	rec    *records.Service
	seen   map[End]bool
	titles map[End]any
}

func newVisibility(rec *records.Service) *visibility {
	return &visibility{rec: rec, seen: map[End]bool{}, titles: map[End]any{}}
}

func (v *visibility) readable(ctx context.Context, e End) bool {
	if ok, done := v.seen[e]; done {
		return ok
	}
	r, err := v.rec.Get(ctx, e.Type, e.ID)
	ok := err == nil
	v.seen[e] = ok
	if ok {
		if t, terr := v.rec.Type(ctx, e.Type); terr == nil {
			if t.TitleField != "" {
				v.titles[e] = r[t.TitleField]
			} else if len(t.Fields) > 0 {
				v.titles[e] = r[t.Fields[0].Name]
			}
		}
	}
	return ok
}

func (v *visibility) title(ctx context.Context, e End) any {
	v.readable(ctx, e)
	return v.titles[e]
}

func textOrNull(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
