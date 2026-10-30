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
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// --- roles at the record (WF3; Inventar G37, G79; architektur.md B1) ---
//
// A type with the trait actors names its roles under actors (requester, observer, assigned,
// …) and which types may hold each. The holders live in the core table actor; a record shows
// them as "actors": {role: [{"type", "id"}]}, every role of the type present, the holders in
// the order of type and id. They are written with the record (create, PATCH) and belong to its
// version: a changed role is an audit row "actors.<role>" with the old and the new list, and
// the event names it under changed. The team of a record is the set of its actors. Filters
// take ids or @me and @my_groups, the person of the session and their groups.

// actorsField is the key of the roles in a record and the name field rights use for them.
const actorsField = "actors"

// The words a filter on actors takes besides ids.
const (
	filterMe       = "@me"
	filterMyGroups = "@my_groups"
)

type actorRef struct {
	Type string
	ID   uuid.UUID
}

// hasActors reports whether the type carries roles at the record.
func hasActors(t meta.Type) bool { return t.HasTrait(meta.TraitActors) && len(t.Actors) > 0 }

// isActorChange reports whether an audit change is a role of the record, not a field or a
// translation ("actors.assigned").
func isActorChange(c change) bool { return strings.HasPrefix(c.field, actorsField+".") }

// actorsExpr is the derived column actors: one subquery per row, so that a read stays one
// round trip. The type name is a validated identifier and safe as a literal.
func actorsExpr(t meta.Type) string {
	return fmt.Sprintf(`(SELECT coalesce(jsonb_object_agg(r.role, r.refs), '{}'::jsonb) FROM (
  SELECT a.role, jsonb_agg(jsonb_build_object('type', a.target_type, 'id', a.target_id) ORDER BY a.target_type, a.target_id) AS refs
  FROM actor a WHERE a.type_name = '%s' AND a.record_id = %s.id GROUP BY a.role) r) AS actors`, t.QualifiedName(), table(t))
}

// selected is the SELECT list of the columns: quoted names, the derived actors as subquery.
func selected(t meta.Type, cols []string) []string {
	out := quoted(cols)
	for i, c := range cols {
		if c == actorsField && hasActors(t) {
			out[i] = actorsExpr(t)
		}
	}
	return out
}

// actorsJSON turns the derived column into the API shape: every role of the type, an empty
// list when nobody holds it.
func actorsJSON(t meta.Type, v any) map[string]any {
	in, _ := v.(map[string]any)
	out := make(map[string]any, len(t.Actors))
	for _, r := range t.Actors {
		list, _ := in[r.Name].([]any)
		if list == nil {
			list = []any{}
		}
		out[r.Name] = list
	}
	return out
}

// refsJSON is the API shape of the holders of one role, in the order the column has.
func refsJSON(refs []actorRef) []any {
	out := make([]any, len(refs))
	for i, r := range refs {
		out[i] = map[string]any{"type": r.Type, "id": r.ID.String()}
	}
	return out
}

// actorsOf parses the roles of the input when the caller gave any.
func (s *Service) actorsOf(ctx context.Context, t meta.Type, raw any, given bool) (map[string][]actorRef, []Problem, error) {
	if !given {
		return nil, nil, nil
	}
	return s.parseActors(ctx, t, raw)
}

// splitActors takes the roles out of the input; what stays goes through validate.
func splitActors(input map[string]any) (map[string]any, any, bool) {
	raw, given := input[actorsField]
	if !given {
		return input, nil, false
	}
	out := make(map[string]any, len(input)-1)
	for k, v := range input {
		if k != actorsField {
			out[k] = v
		}
	}
	return out, raw, true
}

// parseActors checks the roles a caller sets: roles of the type, holders of an allowed type
// that exist, one holder for a single role. null or [] clears a role; a role not named stays
// as it is. The holders come back sorted and without repeats.
func (s *Service) parseActors(ctx context.Context, t meta.Type, raw any) (map[string][]actorRef, []Problem, error) {
	if !hasActors(t) {
		return nil, []Problem{{Field: actorsField, Code: "unknown", Message: t.QualifiedName() + " has no roles at the record (trait actors)"}}, nil
	}
	roles, ok := raw.(map[string]any)
	if !ok {
		return nil, []Problem{{Field: actorsField, Code: "type", Message: `must be an object of roles, like {"assigned": [{"type": "core.group", "id": "…"}]}`}}, nil
	}
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	var ps []Problem
	out := make(map[string][]actorRef, len(roles))
	for _, name := range names {
		field := actorsField + "." + name
		role, ok := t.Actor(name)
		if !ok {
			ps = append(ps, Problem{Field: field, Code: "unknown", Message: fmt.Sprintf("%q is not a role of %s", name, t.QualifiedName())})
			continue
		}
		var list []any
		switch v := roles[name].(type) {
		case nil:
		case []any:
			list = v
		default:
			ps = append(ps, Problem{Field: field, Code: "type", Message: `must be a list of {"type", "id"}`})
			continue
		}
		refs := []actorRef{}
		seen := map[actorRef]bool{}
		bad := false
		for _, item := range list {
			obj, _ := item.(map[string]any)
			typ, _ := obj["type"].(string)
			idStr, _ := obj["id"].(string)
			id, err := uuid.Parse(idStr)
			if obj == nil || typ == "" || err != nil {
				ps = append(ps, Problem{Field: field, Code: "type", Message: `each holder must be {"type": "package.type", "id": "<uuid>"}`})
				bad = true
				break
			}
			if !role.Allows(typ) {
				ps = append(ps, Problem{Field: field, Code: "type", Message: fmt.Sprintf("%s cannot hold the role %s; allowed: %s", typ, name, strings.Join(role.To, ", "))})
				bad = true
				break
			}
			known, err := s.refKnown(ctx, typ, id)
			if err != nil {
				return nil, nil, err
			}
			if !known {
				ps = append(ps, Problem{Field: field, Code: "reference", Message: fmt.Sprintf("no %s with id %s", typ, id)})
				bad = true
				break
			}
			ref := actorRef{Type: typ, ID: id}
			if !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
		if bad {
			continue
		}
		if role.Single && len(refs) > 1 {
			ps = append(ps, Problem{Field: field, Code: "single", Message: "takes one holder only"})
			continue
		}
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Type != refs[j].Type {
				return refs[i].Type < refs[j].Type
			}
			return refs[i].ID.String() < refs[j].ID.String()
		})
		out[name] = refs
	}
	return out, ps, nil
}

// actorChanges compares the holders a caller set with those of the record (nil for a new
// one): one change per role that differs, in the order of the type.
func actorChanges(t meta.Type, old Record, set map[string][]actorRef) []change {
	var before map[string]any
	if old != nil {
		before, _ = old[actorsField].(map[string]any)
	}
	var out []change
	for _, r := range t.Actors {
		refs, ok := set[r.Name]
		if !ok {
			continue
		}
		was, _ := before[r.Name].([]any)
		if was == nil {
			was = []any{}
		}
		now := refsJSON(refs)
		if !equalJSON(was, now) {
			out = append(out, change{actorsField + "." + r.Name, was, now})
		}
	}
	return out
}

// writeActors replaces the holders of every changed role.
func writeActors(ctx context.Context, q *queries.Queries, t meta.Type, id uuid.UUID, set map[string][]actorRef, changes []change, actor Actor) error {
	for _, c := range changes {
		if !isActorChange(c) {
			continue
		}
		role := strings.TrimPrefix(c.field, actorsField+".")
		if err := q.DeleteActorsOfRole(ctx, queries.DeleteActorsOfRoleParams{TypeName: t.QualifiedName(), RecordID: id, Role: role}); err != nil {
			return fmt.Errorf("records: actors: %w", err)
		}
		for _, ref := range set[role] {
			p := queries.InsertActorParams{TypeName: t.QualifiedName(), RecordID: id, Role: role, TargetType: ref.Type, TargetID: ref.ID}
			if actor != nil {
				p.CreatedBy = uuid.NullUUID{UUID: *actor, Valid: true}
			}
			if err := q.InsertActor(ctx, p); err != nil {
				return fmt.Errorf("records: actors: %w", err)
			}
		}
	}
	return nil
}

// withActors adds the roles to the values a rights check sees (the scope involved, the field
// rights): the holders as they will be after the write.
func withActors(values map[string]any, old Record, t meta.Type, set map[string][]actorRef) map[string]any {
	if set == nil {
		return values
	}
	roles := map[string]any{}
	if old != nil {
		if before, ok := old[actorsField].(map[string]any); ok {
			for k, v := range before {
				roles[k] = v
			}
		}
	}
	for name, refs := range set {
		roles[name] = refsJSON(refs)
	}
	out := make(map[string]any, len(values)+1)
	for k, v := range values {
		out[k] = v
	}
	out[actorsField] = actorsJSON(t, roles)
	return out
}

// actorCondition is the SQL for a filter on the holders of a role ("actors.assigned") or of
// any role ("actors"): ids, @me or @my_groups. self names the record's table or alias; arg adds
// an argument and returns its placeholder.
func actorCondition(ctx context.Context, t meta.Type, name string, values []string, self string, arg func(any) string) (string, *Problem) {
	role, p := filterRole(t, name)
	if p != nil {
		return "", p
	}
	a := authz.From(ctx)
	ids := []uuid.UUID{}
	for _, v := range values {
		switch v = strings.TrimSpace(v); v {
		case filterMe:
			if me := a.Me(); me != nil {
				ids = append(ids, *me)
			}
		case filterMyGroups:
			ids = append(ids, a.MyGroups()...)
		default:
			id, err := uuid.Parse(v)
			if err != nil {
				return "", &Problem{Field: name, Code: "type", Message: "must be the id of a holder, @me or @my_groups"}
			}
			ids = append(ids, id)
		}
	}
	cond := "act.type_name = " + arg(t.QualifiedName())
	if role != "" {
		cond += " AND act.role = " + arg(role)
	}
	return fmt.Sprintf("%s.id IN (SELECT act.record_id FROM actor act WHERE %s AND act.target_id = ANY(%s::uuid[]))", self, cond, arg(ids)), nil
}

// actorHeld is the SQL for "somebody holds the role" ("actors.assigned") or any role
// ("actors"); NOT before it means nobody does.
func actorHeld(t meta.Type, name, self string, arg func(any) string) (string, *Problem) {
	role, p := filterRole(t, name)
	if p != nil {
		return "", p
	}
	cond := "act.type_name = " + arg(t.QualifiedName())
	if role != "" {
		cond += " AND act.role = " + arg(role)
	}
	return fmt.Sprintf("EXISTS (SELECT 1 FROM actor act WHERE %s AND act.record_id = %s.id)", cond, self), nil
}

// filterRole is the role a filter name means: "" for "actors" (any role).
func filterRole(t meta.Type, name string) (string, *Problem) {
	if !hasActors(t) {
		return "", &Problem{Field: name, Code: "unknown", Message: t.QualifiedName() + " has no roles at the record (trait actors)"}
	}
	if name == actorsField {
		return "", nil
	}
	role := strings.TrimPrefix(name, actorsField+".")
	if _, ok := t.Actor(role); !ok {
		return "", &Problem{Field: name, Code: "unknown", Message: fmt.Sprintf("%q is not a role of %s", role, t.QualifiedName())}
	}
	return role, nil
}

// isActorFilter reports whether a filter name means the roles at the record.
func isActorFilter(name string) bool {
	return name == actorsField || strings.HasPrefix(name, actorsField+".")
}
