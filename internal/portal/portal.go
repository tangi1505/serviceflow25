// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package portal serves what the portal shows beside the records of the person: the notices
// under "Aktuell" (SD5, SD-Plan N3). A notice is a record of core.portal_notice in the tenant:
// a type, a condition as a JSON query (E40) and the text field shown; the package incident
// brings one for the open major incidents as start data, and each tenant changes it. Whoever
// is signed in reads the notices; the records behind them are read with the rights of the
// system of the tenant, and only the text, the state and the times leave.
package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// TypeNotice is the core type of the notices.
const TypeNotice = "core.portal_notice"

// PerNotice bounds the records one notice shows, the newest first.
const PerNotice = 10

// Item is one record a notice shows.
type Item struct {
	Notice     uuid.UUID  `json:"notice"`
	TypeName   string     `json:"type_name"`
	ID         uuid.UUID  `json:"id"`
	Text       string     `json:"text"`
	State      string     `json:"state,omitempty"`
	StateLabel meta.Label `json:"state_label,omitempty"`
	Since      any        `json:"since"`
	UpdatedAt  any        `json:"updated_at"`
}

// Service reads the notices.
type Service struct{ rec *records.Service }

// New returns the service.
func New(rec *records.Service) *Service { return &Service{rec: rec} }

// Notices returns what the active notices of the tenant show, notice by notice in name order.
// A notice whose type or condition no longer fits is left out instead of failing the portal.
func (s *Service) Notices(ctx context.Context) ([]Item, error) {
	system := authz.WithAccess(ctx, authz.SystemIn(authz.From(ctx).CurrentTenant()))
	page, _, err := s.rec.Query(system, TypeNotice, records.Query{Where: json.RawMessage(`{"field": "active", "eq": true}`), Sort: []string{"name"}, Limit: 50})
	if err != nil {
		return nil, err
	}
	out := []Item{}
	for _, n := range page.Items {
		items, err := s.of(system, n)
		if err != nil {
			continue
		}
		out = append(out, items...)
	}
	return out, nil
}

func (s *Service) of(ctx context.Context, n records.Record) ([]Item, error) {
	notice, err := uuid.Parse(fmt.Sprint(n["id"]))
	if err != nil {
		return nil, err
	}
	target, _ := n["target_type"].(string)
	field, _ := n["text_field"].(string)
	t, err := s.rec.Type(ctx, target)
	if err != nil {
		return nil, err
	}
	q := records.Query{Fields: []string{field, "created_at", "updated_at"}, Sort: []string{"-updated_at"}, Limit: PerNotice}
	if t.HasStates() {
		q.Fields = append(q.Fields, "state")
	}
	if where, _ := n["query"].(string); strings.TrimSpace(where) != "" {
		q.Where = json.RawMessage(where)
	}
	page, _, err := s.rec.Query(ctx, target, q)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(page.Items))
	for _, r := range page.Items {
		text, _ := r[field].(string)
		id, err := uuid.Parse(fmt.Sprint(r["id"]))
		if strings.TrimSpace(text) == "" || err != nil {
			continue
		}
		it := Item{Notice: notice, TypeName: target, ID: id, Text: text, Since: r["created_at"], UpdatedAt: r["updated_at"]}
		if state, ok := r["state"].(string); ok {
			it.State = state
			for _, st := range t.States {
				if st.Name == state {
					it.StateLabel = st.Label
				}
			}
		}
		out = append(out, it)
	}
	return out, nil
}

// Check is the RecordCheck of the notices: the type is installed, the text field is a text
// field of it, and the condition runs (once, one row, with the rights of the writer).
func Check(rec *records.Service) records.RecordCheck {
	return func(ctx context.Context, t meta.Type, r map[string]any) []records.Problem {
		if t.QualifiedName() != TypeNotice {
			return nil
		}
		var ps []records.Problem
		add := func(field, code, msg string) {
			ps = append(ps, records.Problem{Field: field, Code: code, Message: msg})
		}
		name, _ := r["target_type"].(string)
		target, err := rec.Type(ctx, name)
		if err != nil {
			add("target_type", "unknown", fmt.Sprintf("type %s is not installed", name))
			return ps
		}
		field, _ := r["text_field"].(string)
		if f, ok := target.Field(field); !ok || f.Type != meta.FieldText {
			add("text_field", "unknown", fmt.Sprintf("%s has no text field %q", name, field))
		}
		if q, _ := r["query"].(string); strings.TrimSpace(q) != "" {
			var where map[string]any
			if err := json.Unmarshal([]byte(q), &where); err != nil {
				add("query", "type", "must be the where of a JSON query, an object like {\"field\": \"state\", \"eq\": \"new\"}")
			} else if _, _, err := rec.Query(ctx, name, records.Query{Where: json.RawMessage(q), Limit: 1}); err != nil {
				var verr *records.ValidationError
				if !errors.As(err, &verr) {
					return append(ps, records.Problem{Field: "query", Code: "invalid", Message: err.Error()})
				}
				for _, p := range verr.Problems {
					add("query", p.Code, fmt.Sprintf("%s: %s", p.Field, p.Message))
				}
			}
		}
		return ps
	}
}
