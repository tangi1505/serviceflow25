// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package snippets fills in the text modules (SD8; Inventar G56). A text module is a record of
// the core type core.snippet: formatted text for an answer, a solution or a task with the
// placeholders of the notifications (Hauptplan E120). The editor lists them like any records;
// this package checks the placeholders when one is saved and fills them in against a record,
// with the rights of the one who writes: a field they cannot read stays empty.
package snippets

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/notify"
	"group24.de/itsm/internal/records"
)

// TypeSnippet is the core type of the text modules.
const TypeSnippet = "core.snippet"

// Placeholders are the placeholders of a text module besides {{record.<field>}}: there is no
// event, so no {{message}} and no {{entry}}; {{actor}} is the one who writes.
var Placeholders = []string{"type", "title", "link", "actor"}

// Check is the RecordCheck of the text modules: the type, if one is named, is installed, and
// every placeholder exists; {{record.<field>}} without a type in some installed type.
func Check(rec *records.Service) records.RecordCheck {
	return func(ctx context.Context, t meta.Type, r map[string]any) []records.Problem {
		if t.QualifiedName() != TypeSnippet {
			return nil
		}
		name, _ := r["target_type"].(string)
		body, _ := r["body"].(string)
		where, hasField := "", func(string) bool { return false }
		if name != "" {
			target, err := rec.Type(ctx, name)
			if err != nil {
				return []records.Problem{{Field: "target_type", Code: "unknown", Message: fmt.Sprintf("type %s is not installed", name)}}
			}
			where, hasField = name, func(f string) bool { _, ok := target.Field(f); return ok }
		} else if types, err := rec.Types(ctx); err == nil {
			hasField = func(f string) bool {
				for _, it := range types {
					if _, ok := it.Field(f); ok {
						return true
					}
				}
				return false
			}
		}
		if msg := notify.CheckPlaceholders(body, Placeholders, where, hasField); msg != "" {
			return []records.Problem{{Field: "body", Code: "type", Message: msg}}
		}
		return nil
	}
}

// Service fills in the text modules.
type Service struct {
	rec *records.Service
	// PublicURL is the address of the web interface, for {{link}}.
	PublicURL string
}

// New returns the service.
func New(rec *records.Service) *Service { return &Service{rec: rec} }

// Render returns the text of a text module filled in against a record, as formatted text: the
// values escaped, a reference by its title, {{link}} a link. Text module and record are read
// with the rights of the caller; a text module for another type is refused.
func (s *Service) Render(ctx context.Context, snippet uuid.UUID, typeName string, id uuid.UUID) (string, error) {
	sn, err := s.rec.Get(ctx, TypeSnippet, snippet)
	if err != nil {
		return "", err
	}
	if target, _ := sn["target_type"].(string); target != "" && target != typeName {
		return "", &records.ValidationError{Problems: []records.Problem{{Field: "snippet", Code: "type",
			Message: fmt.Sprintf("the text module is for %s, not %s", target, typeName)}}}
	}
	t, err := s.rec.Type(ctx, typeName)
	if err != nil {
		return "", err
	}
	r, err := s.rec.Get(ctx, typeName, id)
	if err != nil {
		return "", err
	}
	lang, err := s.rec.Localize(ctx, typeName, r)
	if err != nil {
		return "", err
	}
	body, _ := sn["body"].(string)
	return notify.Render(body, notify.Data{
		Type: t, Record: r, Lang: lang, HTML: true,
		Link:  fmt.Sprintf("%s/types/%s/%s", s.PublicURL, typeName, id),
		Actor: s.writer(ctx),
		Name:  func(to, ref string) string { return s.title(ctx, to, ref) },
	}), nil
}

// writer is the name of the person who writes; their own name is read as the system of the
// tenant, since not every role may read persons.
func (s *Service) writer(ctx context.Context) string {
	a := authz.From(ctx)
	if a == nil || a.PersonID == nil {
		return ""
	}
	sys := ctx
	if !a.IsSystem() {
		sys = authz.WithAccess(ctx, authz.SystemIn(a.CurrentTenant()))
	}
	p, err := s.rec.Get(sys, "core.person", *a.PersonID)
	if err != nil {
		return ""
	}
	first, _ := p["first_name"].(string)
	last, _ := p["last_name"].(string)
	switch {
	case first == "":
		return last
	case last == "":
		return first
	}
	return first + " " + last
}

// title is the title of a referenced record in the language of the request, empty when the
// caller may not read it.
func (s *Service) title(ctx context.Context, typeName, ref string) string {
	id, err := uuid.Parse(ref)
	if err != nil {
		return ""
	}
	t, err := s.rec.Type(ctx, typeName)
	if err != nil || t.TitleField == "" {
		return ""
	}
	r, err := s.rec.Get(ctx, typeName, id)
	if err != nil {
		return ""
	}
	if _, err := s.rec.Localize(ctx, typeName, r); err != nil {
		return ""
	}
	if v := r[t.TitleField]; v != nil {
		return fmt.Sprint(v)
	}
	return ""
}
