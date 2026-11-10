// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package suggest offers records while another record is written (SD18, SD-Plan E12, N13):
// articles while a user reports a fault in the portal, and in the tab of a ticket. A type
// says under suggest for which types it is offered and which of their fields are searched;
// the full-text search runs with any word counting, with the rights of the session. Each hit
// comes with its rating, how often it was linked and whether the record is linked to it.
package suggest

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/relations"
)

// Request is what the form sends: its values so far, or a text of its own; the id when the
// record exists.
type Request struct {
	Values map[string]any
	Text   string
	ID     *uuid.UUID
	Limit  int // hits per group, default 5, at most 20
}

// Group is the suggestions of one type.
type Group struct {
	Type     string     `json:"type"`
	Label    meta.Label `json:"label"`
	Relation string     `json:"relation,omitempty"`
	Apply    *Apply     `json:"apply,omitempty"`
	Hits     []Hit      `json:"hits"`
}

// Apply is what using a hit sets in the form: "@<field>" the hit's field, a select list's
// entry already as its id.
type Apply struct {
	Label meta.Label     `json:"label"`
	Set   map[string]any `json:"set"`
}

// Hit is one suggestion.
type Hit struct {
	ID      uuid.UUID       `json:"id"`
	Title   *string         `json:"title"`
	Snippet string          `json:"snippet"`
	Rank    float64         `json:"rank"`
	Rating  *records.Rating `json:"rating,omitempty"`
	Uses    *int64          `json:"uses,omitempty"`
	Linked  *uuid.UUID      `json:"linked,omitempty"`
}

// Service finds suggestions.
type Service struct {
	rec *records.Service
	rel *relations.Service
}

// New wires the service.
func New(rec *records.Service, rel *relations.Service) *Service {
	return &Service{rec: rec, rel: rel}
}

// Suggest returns one group per type that suggests itself for records of typeName and that
// the session may read, in the order of their names; a group without hits is there too, so
// that the interface knows where to offer an own search.
func (s *Service) Suggest(ctx context.Context, typeName string, req Request) ([]Group, error) {
	target, err := s.rec.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	limit = min(limit, 20)
	types, err := s.rec.Types(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(types, func(i, j int) bool { return types[i].QualifiedName() < types[j].QualifiedName() })
	a := authz.From(ctx)
	out := []Group{}
	for _, t := range types {
		sg := t.Suggest
		if sg == nil || !sg.Suggests(target) || !a.Allowed(t.QualifiedName(), authz.ActionRead) {
			continue
		}
		g := Group{Type: t.QualifiedName(), Label: sg.Label, Relation: sg.Relation, Hits: []Hit{}}
		if g.Apply, err = s.apply(ctx, target, sg.Apply); err != nil {
			return nil, err
		}
		text := strings.TrimSpace(req.Text)
		if text == "" {
			text = textOf(target, sg.From, req.Values)
		}
		found, err := s.rec.Search(ctx, records.SearchQuery{Text: text, Types: []string{g.Type}, Limit: limit, Any: true})
		if err != nil {
			return nil, err
		}
		if err := s.fill(ctx, &g, t, found, typeName, req.ID); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

// textOf joins the values of the fields the suggestion searches with that the target has.
func textOf(target meta.Type, from []string, values map[string]any) string {
	var parts []string
	for _, name := range from {
		if _, ok := target.Field(name); !ok {
			continue
		}
		if v, ok := values[name].(string); ok && strings.TrimSpace(v) != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

// apply resolves what using a hit sets, for a target that has every field of it; nil else.
func (s *Service) apply(ctx context.Context, target meta.Type, ap *meta.SuggestApply) (*Apply, error) {
	if ap == nil {
		return nil, nil
	}
	set := make(map[string]any, len(ap.Set))
	for name, v := range ap.Set {
		if _, ok := target.Field(name); !ok {
			return nil, nil
		}
		set[name] = v
	}
	resolved, err := s.rec.ListKeys(ctx, target, set)
	if err != nil {
		return nil, fmt.Errorf("suggest: apply for %s: %w", target.QualifiedName(), err)
	}
	return &Apply{Label: ap.Label, Set: resolved}, nil
}

// fill turns the search hits into the group's hits with rating, uses and the relation that
// links the record to a hit.
func (s *Service) fill(ctx context.Context, g *Group, t meta.Type, found []records.SearchHit, typeName string, id *uuid.UUID) error {
	ids := make([]uuid.UUID, len(found))
	for i, h := range found {
		ids[i] = h.ID
		g.Hits = append(g.Hits, Hit{ID: h.ID, Title: h.Title, Snippet: h.Snippet, Rank: h.Rank})
	}
	if len(ids) == 0 {
		return nil
	}
	if t.HasTrait(meta.TraitRating) {
		sums, err := s.rec.Ratings(ctx, g.Type, ids)
		if err != nil {
			return err
		}
		for i := range g.Hits {
			r := sums[g.Hits[i].ID]
			g.Hits[i].Rating = &r
		}
	}
	if g.Relation == "" {
		return nil
	}
	uses, err := s.rel.UsesOf(ctx, g.Relation, g.Type, ids)
	if err != nil {
		return err
	}
	linked := map[uuid.UUID]uuid.UUID{}
	if id != nil {
		rels, err := s.rel.Of(ctx, relations.End{Type: typeName, ID: *id}, g.Relation, "out")
		if err != nil {
			return err
		}
		for _, r := range rels {
			if r.To.Type == g.Type {
				linked[r.To.ID] = r.ID
			}
		}
	}
	for i := range g.Hits {
		n := uses[g.Hits[i].ID]
		g.Hits[i].Uses = &n
		if r, ok := linked[g.Hits[i].ID]; ok {
			g.Hits[i].Linked = &r
		}
	}
	return nil
}
