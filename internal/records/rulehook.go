// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
)

// --- the hooks of the rule engine (WF4; architektur.md §6.2) ---
//
// A rule "before" changes a record before it is written: records asks the engine (Before)
// with the record as it will be and the fields the write changes, and merges what the rules
// set into the same write, with the rights of the system: neither the caller's field rights
// nor read-only field states hold rules back, the record's own checks do. The history names
// the rule of every change it made. A rule "after" writes through the usual calls; its context
// carries the rule and the depth of the chain (WithRule), which the history and the event
// carry on, so that the engine knows who wrote and stops loops.

// Trigger names the write a rule reacts to.
const (
	TriggerCreate = "create"
	TriggerUpdate = "update"
)

// RuleChange is what the rules before a write add to it.
type RuleChange struct {
	// Set holds field values in the API shape.
	Set map[string]any
	// Actors holds whole roles in the API shape ([{"type", "id"}]); a role named replaces it.
	Actors map[string]any
	// By names the rule of every field and role ("actors.<role>") it set.
	By map[string]string
}

// BeforeWrite is the hook of the engine: the record as it will be (API shape) and the names
// of the fields the write changes ("actors.<role>" for roles).
type BeforeWrite func(ctx context.Context, t meta.Type, trigger string, rec map[string]any, changed []string) (RuleChange, error)

type ruleKey struct{}

// RuleTag is the rule a write in the context comes from, and how deep in a chain of rules.
type RuleTag struct {
	Name  string
	Depth int
}

// WithRule marks the writes in the context as made by a rule (WF4).
func WithRule(ctx context.Context, name string, depth int) context.Context {
	return context.WithValue(ctx, ruleKey{}, RuleTag{Name: name, Depth: depth})
}

// RuleOf returns the rule of the context, if any.
func RuleOf(ctx context.Context) (RuleTag, bool) {
	t, ok := ctx.Value(ruleKey{}).(RuleTag)
	return t, ok
}

// withRuleName tags one audit row with a rule, keeping the depth of the context.
func withRuleName(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}
	tag, _ := RuleOf(ctx)
	return WithRule(ctx, name, tag.Depth)
}

// applyRules asks the engine before a write and merges its changes: values (checked like the
// caller's) and roles. It returns the names the rules set, for the field states and the
// history.
func (s *Service) applyRules(ctx context.Context, t meta.Type, trigger string, rec map[string]any, values map[string]any,
	actors map[string][]actorRef, changed []string) (map[string][]actorRef, map[string]string, error) {
	if s.Before == nil {
		return actors, nil, nil
	}
	rc, err := s.Before(ctx, t, trigger, rec, changed)
	if err != nil {
		return nil, nil, err
	}
	if len(rc.Set) == 0 && len(rc.Actors) == 0 {
		return actors, nil, nil
	}
	if len(rc.Set) > 0 {
		set, err := s.validate(ctx, t, rc.Set, false)
		if err != nil {
			return nil, nil, ruleError(err, rc.By)
		}
		for k, v := range set {
			values[k] = v
		}
	}
	if len(rc.Actors) > 0 {
		parsed, ps, err := s.parseActors(ctx, t, rc.Actors)
		if err != nil {
			return nil, nil, err
		}
		if len(ps) > 0 {
			return nil, nil, ruleError(&ValidationError{Problems: ps}, rc.By)
		}
		if actors == nil {
			actors = map[string][]actorRef{}
		}
		for role, refs := range parsed {
			actors[role] = refs
		}
	}
	return actors, rc.By, nil
}

// ruleError names the rule in the problems of values a rule set: the definition is wrong, not
// the caller's input.
func ruleError(err error, by map[string]string) error {
	ve, ok := AsValidationError(err)
	if !ok {
		return err
	}
	for i, p := range ve.Problems {
		if name := by[p.Field]; name != "" {
			ve.Problems[i].Message = fmt.Sprintf("rule %s: %s", name, p.Message)
		}
	}
	return ve
}

// rulesWould tells the bulk whether rules before the write change a row; such rows go the
// way of Sync (an error of the engine counts as yes, Sync then reports it).
func (s *Service) rulesWould(ctx context.Context, t meta.Type, trigger string, rec map[string]any, changed []string) bool {
	if s.Before == nil {
		return false
	}
	rc, err := s.Before(ctx, t, trigger, rec, changed)
	return err != nil || len(rc.Set) > 0 || len(rc.Actors) > 0
}

// changedNames lists the fields a create sets, or an update changes, and the roles.
func changedNames(t meta.Type, old Record, values map[string]any, actorCh []change) []string {
	out := []string{}
	for _, f := range t.Fields {
		v, ok := values[f.Name]
		if !ok {
			continue
		}
		if old == nil || !equalJSON(old[f.Name], toJSON(f, v)) {
			out = append(out, f.Name)
		}
	}
	for _, c := range actorCh {
		out = append(out, c.field)
	}
	return out
}

// CheckValues checks values against a type as a create (all required fields) or a change
// would, without writing; roles under "actors" too. The rule engine checks its actions with it.
func (s *Service) CheckValues(ctx context.Context, typeName string, values map[string]any, create bool) error {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return err
	}
	values, raw, given := splitActors(values)
	_, aps, err := s.actorsOf(ctx, t, raw, given)
	if err != nil {
		return err
	}
	_, err = s.validate(ctx, t, values, create)
	return withProblems(err, aps)
}

// Matching lists up to limit records of a type (outside the trash) the CEL condition holds
// for, translated to SQL like the conditions of permissions (PL9); the rules on a schedule
// find their records with it (WF4). An empty condition matches every record; under keeps
// the records below that node (the tenant of the rule, PL45), empty means all.
func (s *Service) Matching(ctx context.Context, typeName, condition, under string, limit int) ([]uuid.UUID, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	where := []string{"deleted_at IS NULL"}
	if c := listScope(t, ""); c != "" {
		where = append(where, c)
	}
	var args []any
	if under != "" {
		// A rule of a tenant finds only its records (PL45).
		args = append(args, under)
		where = append(where, "org_path <@ $1::ltree")
	}
	if strings.TrimSpace(condition) != "" {
		cond, a, err := authz.Translate(condition, args)
		if err != nil {
			return nil, err
		}
		where, args = append(where, cond), a
	}
	sql := fmt.Sprintf("SELECT id FROM %s WHERE %s ORDER BY id LIMIT %d", table(t), strings.Join(where, " AND "), limit)
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("records: matching %s: %w", typeName, err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// rulesOf is the part of an event that names the rules before the write, or nil.
func rulesOf(by map[string]string) map[string]any {
	if len(by) == 0 {
		return nil
	}
	return map[string]any{"rules": ruleNames(by)}
}

// ruleNames lists the rules of a write's changes, for the event.
func ruleNames(by map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range by {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
