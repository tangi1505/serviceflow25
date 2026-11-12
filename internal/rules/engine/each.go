// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package engine

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// --- records of a template (SD6, SD-Plan N17, N18) ---
//
// A rule may act once for each record of another type that belongs to this one: the lines of
// a template the record names. Each says which type, which of its fields must equal which
// field of the record ({"template": "template"}: the lines whose field template names the same
// as the record's field template) and by which field they come in order. "create" with each
// makes one record per line, "@item.<field>" in its fields and holders taking the line's value;
// "approval" asks for approval with one stage per line (SD6: the stages of the template).

// Each names the records a rule acts on once each.
type Each struct {
	TypeName string            `json:"type_name"`
	Match    map[string]string `json:"match"`
	Order    string            `json:"order,omitempty"`
}

// ApprovalAction asks for approval of the record with one stage per record of Stages (SD6):
// its name from the field Name, its least number of yes from the field Need (empty: all), its
// approvers from the role Approvers. Lines without approvers are left out; without any line
// nothing is asked.
//
// With Role instead of Stages the holders of that role at the record itself approve, in one
// stage where one yes is enough (SD17: the editors of an article).
//
// With Template instead the stages come from a template of approvals (WF7, WF-Plan E18): the
// id of a core.approval_template, or a path of reference fields from the record to one
// ("template.approval_template": the approval template of the record's fulfilment template).
// Message, when set, replaces the template's text.
type ApprovalAction struct {
	Stages    Each   `json:"stages"`
	Name      string `json:"name,omitempty"`
	Need      string `json:"need,omitempty"`
	Approvers string `json:"approvers,omitempty"`
	Role      string `json:"role,omitempty"`
	Template  string `json:"template,omitempty"`
	Message   string `json:"message,omitempty"`
}

// MaxEach bounds the records a rule acts on once each.
const MaxEach = 200

// placeItem is the line a rule acts on: "@item" its id, "@item.<field>" a value of it.
const placeItem = "@item"

// checkEach checks the type, the matched fields and the order against the rule's type.
func (e *Engine) checkEach(ctx context.Context, typeName string, x Each) (meta.Type, string) {
	t, err := e.rec.Type(ctx, typeName)
	if err != nil {
		return meta.Type{}, err.Error()
	}
	row, err := e.rec.Type(ctx, x.TypeName)
	if err != nil {
		return meta.Type{}, "each: " + err.Error()
	}
	if len(x.Match) == 0 {
		return row, "each: match names at least one field of " + x.TypeName + " and the field of " + typeName + " it must equal"
	}
	for own, other := range x.Match {
		if _, ok := row.Field(own); !ok {
			return row, fmt.Sprintf("each: match: %q is not a field of %s", own, x.TypeName)
		}
		if _, ok := t.Field(other); !ok && other != "id" {
			return row, fmt.Sprintf("each: match: %q is not a field of %s", other, typeName)
		}
	}
	if x.Order != "" {
		if _, ok := row.Field(x.Order); !ok {
			return row, fmt.Sprintf("each: order: %q is not a field of %s", x.Order, x.TypeName)
		}
	}
	return row, ""
}

// items reads the records of each that belong to the record, in their order; none when a
// matched field of the record is empty.
func (e *Engine) items(ctx context.Context, rec map[string]any, x Each) ([]records.Record, error) {
	filter := map[string]string{}
	for own, other := range x.Match {
		v := rec[other]
		if v == nil || v == "" {
			return nil, nil
		}
		filter[own] = fmt.Sprint(v)
	}
	page, err := e.rec.List(ctx, x.TypeName, records.ListOptions{Limit: MaxEach, Filter: filter})
	if err != nil {
		return nil, err
	}
	list := page.Items
	if x.Order != "" {
		slices.SortStableFunc(list, func(a, b records.Record) int {
			fa, oka := a[x.Order].(float64)
			fb, okb := b[x.Order].(float64)
			if oka && okb {
				return cmp.Compare(fa, fb)
			}
			return strings.Compare(fmt.Sprint(a[x.Order]), fmt.Sprint(b[x.Order]))
		})
	}
	return list, nil
}

// itemValue is what "@item…" stands for in a line, or nil.
func itemValue(item records.Record, place string) (any, bool) {
	if place == placeItem {
		return fmt.Sprint(item["id"]), true
	}
	field, ok := strings.CutPrefix(place, placeItem+".")
	if !ok {
		return nil, false
	}
	return item[field], true
}

// bindItem fills "@item…" in the fields and the holders of a record a rule makes for a line; a
// holder whose line has no value is left out, so is a field.
func bindItem(fields map[string]any, item records.Record) map[string]any {
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		if k == "actors" {
			out[k] = bindItemActors(v, item)
			continue
		}
		if s, ok := v.(string); ok {
			if val, isItem := itemValue(item, s); isItem {
				if val == nil {
					continue
				}
				v = val
			}
		}
		out[k] = v
	}
	return out
}

func bindItemActors(v any, item records.Record) any {
	roles, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(roles))
	for role, list := range roles {
		holders, ok := list.([]any)
		if !ok {
			out[role] = list
			continue
		}
		kept := []any{}
		for _, h := range holders {
			m, _ := h.(map[string]any)
			s, _ := m["id"].(string)
			val, isItem := itemValue(item, s)
			if !isItem {
				kept = append(kept, h)
				continue
			}
			if val != nil && val != "" {
				kept = append(kept, map[string]any{"type": m["type"], "id": fmt.Sprint(val)})
			}
		}
		out[role] = kept
	}
	return out
}

// isItemPlace reports whether a value stands for a line.
func isItemPlace(v any) bool {
	s, ok := v.(string)
	return ok && (s == placeItem || strings.HasPrefix(s, placeItem+"."))
}

// checkApproval checks an action approval: the rule's type has approvals, the lines name
// existing fields and a role of persons or groups.
func (e *Engine) checkApproval(ctx context.Context, typeName string, a *ApprovalAction) string {
	t, err := e.rec.Type(ctx, typeName)
	if err != nil {
		return err.Error()
	}
	if !t.HasApprovals() {
		return typeName + " has no approvals (trait approvals)"
	}
	if a.Template != "" {
		if a.Role != "" || a.Stages.TypeName != "" || a.Approvers != "" {
			return "template: either template, role, or stages with approvers"
		}
		return e.checkTemplatePath(ctx, t, a.Template)
	}
	if a.Role != "" {
		if a.Stages.TypeName != "" || a.Approvers != "" {
			return "role: either role, or stages with approvers"
		}
		if role, ok := t.Actor(a.Role); !ok || (!role.Allows("core.person") && !role.Allows("core.group")) {
			return fmt.Sprintf("role: %q is no role of %s for persons or groups", a.Role, typeName)
		}
		return ""
	}
	row, msg := e.checkEach(ctx, typeName, a.Stages)
	if msg != "" {
		return "stages: " + msg
	}
	if role, ok := row.Actor(a.Approvers); !ok || (!role.Allows("core.person") && !role.Allows("core.group")) {
		return fmt.Sprintf("approvers: %q is no role of %s for persons or groups", a.Approvers, a.Stages.TypeName)
	}
	if a.Name != "" {
		if f, ok := row.Field(a.Name); !ok || f.Type != meta.FieldText {
			return fmt.Sprintf("name: %q is no text field of %s", a.Name, a.Stages.TypeName)
		}
	}
	if a.Need != "" {
		if f, ok := row.Field(a.Need); !ok || f.Type != meta.FieldNumber {
			return fmt.Sprintf("need: %q is no number field of %s", a.Need, a.Stages.TypeName)
		}
	}
	return ""
}

// createOnce makes the record of one line once per record the rule acts on, however often the
// rule fires (two jobs may see the record alike when they run late): the key is the rule, the
// record and the line, like a bulk row (PL31). A record made before, also in the trash, stays
// as it is, changed or not.
func (e *Engine) createOnce(ctx context.Context, r Rule, typeName string, id uuid.UUID, line records.Record, fields map[string]any) error {
	key := records.SyncKey{Source: "rule:" + r.Name, Key: id.String() + "/" + fmt.Sprint(line["id"])}
	sum := sha256.Sum256([]byte(key.Source + "|" + key.Key))
	key.Hash = sum[:]
	page, err := e.rec.List(ctx, typeName, records.ListOptions{Limit: 1, Filter: map[string]string{"source": key.Source, "external_key": key.Key}})
	if err != nil {
		return err
	}
	if len(page.Items) > 0 {
		return nil
	}
	_, _, _, err = e.rec.Sync(ctx, typeName, key, fields, nil)
	switch {
	case errors.Is(err, records.ErrConflict):
		// Made meanwhile by the other job, or in the trash.
		return nil
	case err != nil:
		return refused(r, err)
	}
	return nil
}

// checkTemplatePath checks the template of an action approval: an id, or reference fields
// from the record to a core.approval_template.
func (e *Engine) checkTemplatePath(ctx context.Context, t meta.Type, path string) string {
	if _, err := uuid.Parse(path); err == nil {
		return ""
	}
	cur := t
	parts := strings.Split(path, ".")
	for i, name := range parts {
		f, ok := cur.Field(name)
		if !ok || f.Type != meta.FieldReference {
			return fmt.Sprintf("template: %q is no reference field of %s", name, cur.QualifiedName())
		}
		if i == len(parts)-1 {
			if f.To != records.TypeApprovalTemplate {
				return fmt.Sprintf("template: %s.%s refers to %s, not to %s", cur.QualifiedName(), name, f.To, records.TypeApprovalTemplate)
			}
			return ""
		}
		next, err := e.rec.Type(ctx, f.To)
		if err != nil {
			return "template: " + err.Error()
		}
		cur = next
	}
	return ""
}

// templateOf follows the template of an action approval from the record; uuid.Nil when a field
// on the way is empty.
func (e *Engine) templateOf(ctx context.Context, typeName string, rec map[string]any, path string) (uuid.UUID, error) {
	if id, err := uuid.Parse(path); err == nil {
		return id, nil
	}
	cur := rec
	parts := strings.Split(path, ".")
	for i, name := range parts {
		raw, _ := cur[name].(string)
		if raw == "" {
			return uuid.Nil, nil
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, fmt.Errorf("template: %s is no id: %w", name, err)
		}
		if i == len(parts)-1 {
			return id, nil
		}
		t, err := e.rec.Type(ctx, typeName)
		if err != nil {
			return uuid.Nil, err
		}
		f, _ := t.Field(name)
		next, err := e.rec.Get(ctx, f.To, id)
		if errors.Is(err, records.ErrNotFound) {
			return uuid.Nil, nil
		}
		if err != nil {
			return uuid.Nil, err
		}
		cur, typeName = next, f.To
	}
	return uuid.Nil, nil
}

// askApproval asks for approval with the stages from the lines; nothing without one, nothing
// while one is open (two jobs may see the record alike when they run late).
func (e *Engine) askApproval(ctx context.Context, r Rule, typeName string, id uuid.UUID, rec map[string]any, a *ApprovalAction) error {
	if rec[meta.FieldApproval] == meta.ApprovalPending {
		return nil
	}
	req := records.ApprovalRequest{Message: a.Message}
	var lines []records.Record
	if a.Template != "" {
		tid, err := e.templateOf(ctx, typeName, rec, a.Template)
		if err != nil {
			return err
		}
		if tid == uuid.Nil {
			return nil
		}
		if req, err = e.rec.ApprovalFromTemplate(ctx, typeName, id, tid); err != nil {
			if errors.Is(err, records.ErrNotFound) {
				return refused(r, err)
			}
			return err
		}
		if a.Message != "" {
			req.Message = a.Message
		}
	} else if a.Role != "" {
		// The holders of a role of the record itself (SD17): one stage, one yes is enough.
		var approvers []records.Ref
		for _, h := range holdersOf(rec, a.Role) {
			m, _ := h.(map[string]any)
			if hid, err := uuid.Parse(fmt.Sprint(m["id"])); err == nil {
				approvers = append(approvers, records.Ref{Type: fmt.Sprint(m["type"]), ID: hid})
			}
		}
		if len(approvers) > 0 {
			req.Stages = append(req.Stages, records.StageRequest{Need: 1, Approvers: approvers})
		}
	} else {
		var err error
		if lines, err = e.items(ctx, rec, a.Stages); err != nil {
			return err
		}
	}
	for _, line := range lines {
		var approvers []records.Ref
		for _, h := range holdersOf(line, a.Approvers) {
			m, _ := h.(map[string]any)
			hid, err := uuid.Parse(fmt.Sprint(m["id"]))
			if err != nil {
				continue
			}
			approvers = append(approvers, records.Ref{Type: fmt.Sprint(m["type"]), ID: hid})
		}
		if len(approvers) == 0 {
			continue
		}
		st := records.StageRequest{Approvers: approvers}
		if a.Name != "" {
			st.Name, _ = line[a.Name].(string)
		}
		if a.Need != "" {
			if n, ok := line[a.Need].(float64); ok && n > 0 {
				st.Need = min(int(n), len(approvers))
			}
		}
		req.Stages = append(req.Stages, st)
	}
	if len(req.Stages) == 0 {
		return nil
	}
	_, err := e.rec.RequestApproval(ctx, typeName, id, req, nil)
	if errors.Is(err, records.ErrConflict) && strings.Contains(err.Error(), "open approval") {
		// Asked meanwhile by the other job.
		return nil
	}
	if err != nil {
		return refused(r, err)
	}
	return nil
}
