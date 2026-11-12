// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
)

// --- templates of approvals (WF7, WF-Plan E18; Inventar G45) ---
//
// A template (core.approval_template) holds the text to the approvers, the stages
// (core.approval_template_stage) and the working days between reminders. A stage names persons
// and groups, and roles of the record ("actors.<role>", "actors.<role>.responsible" for the
// responsible persons of the groups in the role), resolved when the approval is asked; a stage
// without anybody is left out. Whoever asks starts from a draft and may change it; a rule asks
// from the template as it is. The approval keeps the template and its working days.

// The core types of the templates, the setting and its default.
const (
	TypeApprovalTemplate      = "core.approval_template"
	TypeApprovalTemplateStage = "core.approval_template_stage"
	// SettingRemindDays is the number of working days between reminders of an open stage at a
	// node of the tree; 0 means none.
	SettingRemindDays = "approvals.remind_days"
	DefaultRemindDays = 2
)

// TemplateApprover is a person or group of a draft; Role names the role of the record it
// comes from ("assignee_group.responsible"), empty for one the template names itself.
type TemplateApprover struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
	Role string    `json:"role,omitempty"`
}

// TemplateStage is a stage of a draft.
type TemplateStage struct {
	Name      string             `json:"name"`
	Need      int                `json:"need"`
	Approvers []TemplateApprover `json:"approvers"`
}

// ApprovalDraft is a template filled in for one record: the stages with the roles resolved.
type ApprovalDraft struct {
	Template   uuid.UUID       `json:"template"`
	Name       string          `json:"name"`
	Message    string          `json:"message"`
	RemindDays *int            `json:"remind_days,omitempty"`
	Stages     []TemplateStage `json:"stages"`
}

// Request is the draft as a request, the template named.
func (d ApprovalDraft) Request() ApprovalRequest {
	tid := d.Template
	req := ApprovalRequest{Message: d.Message, Template: &tid}
	for _, st := range d.Stages {
		sr := StageRequest{Name: st.Name, Need: st.Need}
		for _, a := range st.Approvers {
			sr.Approvers = append(sr.Approvers, Ref{Type: a.Type, ID: a.ID})
		}
		req.Stages = append(req.Stages, sr)
	}
	return req
}

// ApprovalTemplates lists the active templates filled in for a record the session may change,
// by name; templates without any stage that has somebody are left out.
func (s *Service) ApprovalTemplates(ctx context.Context, typeName string, id uuid.UUID) ([]ApprovalDraft, error) {
	t, err := s.approvalType(ctx, typeName)
	if err != nil {
		return nil, err
	}
	rec, err := s.Get(ctx, typeName, id)
	if err != nil {
		return nil, err
	}
	if err := checkAccess(ctx, t, rec, authz.ActionUpdate); err != nil {
		return nil, err
	}
	system := systemOf(ctx)
	out := []ApprovalDraft{}
	after := ""
	for {
		page, err := s.List(system, TypeApprovalTemplate, ListOptions{Limit: 500, After: after})
		if err != nil {
			return nil, err
		}
		for _, tpl := range page.Items {
			if tpl["active"] == false {
				continue
			}
			d, err := s.draft(system, rec, tpl)
			if err != nil {
				return nil, err
			}
			if len(d.Stages) > 0 {
				out = append(out, d)
			}
		}
		if page.Next == "" {
			break
		}
		after = page.Next
	}
	slices.SortStableFunc(out, func(a, b ApprovalDraft) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

// ApprovalFromTemplate is the request a template makes for a record, read with the rights of
// the system: a rule asks with it (WF7). Without any stage that has somebody it has no stages.
func (s *Service) ApprovalFromTemplate(ctx context.Context, typeName string, id, template uuid.UUID) (ApprovalRequest, error) {
	system := systemOf(ctx)
	rec, err := s.Get(system, typeName, id)
	if err != nil {
		return ApprovalRequest{}, err
	}
	tpl, err := s.Get(system, TypeApprovalTemplate, template)
	if err != nil {
		return ApprovalRequest{}, fmt.Errorf("approval template %s: %w", template, err)
	}
	d, err := s.draft(system, rec, tpl)
	if err != nil {
		return ApprovalRequest{}, err
	}
	return d.Request(), nil
}

// draft fills a template in for a record.
func (s *Service) draft(ctx context.Context, rec, tpl Record) (ApprovalDraft, error) {
	tid, err := uuid.Parse(fmt.Sprint(tpl["id"]))
	if err != nil {
		return ApprovalDraft{}, err
	}
	d := ApprovalDraft{Template: tid, Stages: []TemplateStage{}}
	d.Name, _ = tpl["name"].(string)
	d.Message, _ = tpl["message"].(string)
	if n, ok := tpl["remind_days"].(float64); ok && n >= 0 {
		days := int(n)
		d.RemindDays = &days
	}
	page, err := s.List(ctx, TypeApprovalTemplateStage, ListOptions{Limit: 500, Filter: map[string]string{"template": tid.String()}})
	if err != nil {
		return ApprovalDraft{}, err
	}
	lines := page.Items
	slices.SortStableFunc(lines, func(a, b Record) int {
		pa, _ := a["position"].(float64)
		pb, _ := b["position"].(float64)
		if c := cmp.Compare(pa, pb); c != 0 {
			return c
		}
		return strings.Compare(fmt.Sprint(a["name"]), fmt.Sprint(b["name"]))
	})
	for _, line := range lines {
		st := TemplateStage{Approvers: []TemplateApprover{}}
		st.Name, _ = line["name"].(string)
		seen := map[Ref]bool{}
		add := func(ref Ref, role string) {
			if !seen[ref] && len(st.Approvers) < MaxApproversPerStage {
				seen[ref] = true
				st.Approvers = append(st.Approvers, TemplateApprover{Type: ref.Type, ID: ref.ID, Role: role})
			}
		}
		for _, ref := range holders(line, "approvers") {
			add(ref, "")
		}
		specs, _ := ParseRoleSpecs(fmt.Sprint(line["roles"]))
		for _, sp := range specs {
			refs, err := s.resolveRole(ctx, rec, sp)
			if err != nil {
				return ApprovalDraft{}, err
			}
			for _, ref := range refs {
				add(ref, sp.String())
			}
		}
		if len(st.Approvers) == 0 {
			continue
		}
		if n, ok := line["need"].(float64); ok && n > 0 {
			st.Need = min(int(n), len(st.Approvers))
		}
		d.Stages = append(d.Stages, st)
		if len(d.Stages) == MaxApprovalStages {
			break
		}
	}
	return d, nil
}

// RoleSpec is a role of a record a template names: who holds it, or with Responsible the
// responsible persons of the groups that hold it.
type RoleSpec struct {
	Role        string
	Responsible bool
}

func (r RoleSpec) String() string {
	if r.Responsible {
		return r.Role + meta.LeadSuffix
	}
	return r.Role
}

// ParseRoleSpecs reads "actors.<role>" and "actors.<role>.responsible", separated by commas;
// an empty text names none.
func ParseRoleSpecs(text string) ([]RoleSpec, error) {
	var out []RoleSpec
	for part := range strings.SplitSeq(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "<nil>" {
			continue
		}
		spec, ok := strings.CutPrefix(part, "actors.")
		if !ok || spec == "" {
			return nil, fmt.Errorf("%q is no role like actors.requester or actors.assignee_group.responsible", part)
		}
		role, lead := strings.CutSuffix(spec, meta.LeadSuffix)
		if role == "" || strings.Contains(role, ".") {
			return nil, fmt.Errorf("%q is no role like actors.requester or actors.assignee_group.responsible", part)
		}
		out = append(out, RoleSpec{Role: role, Responsible: lead})
	}
	return out, nil
}

// resolveRole gives the persons and groups a role of the record stands for now.
func (s *Service) resolveRole(ctx context.Context, rec Record, sp RoleSpec) ([]Ref, error) {
	refs := holders(rec, sp.Role)
	if !sp.Responsible {
		return refs, nil
	}
	var out []Ref
	for _, ref := range refs {
		if ref.Type != "core.group" {
			continue
		}
		g, err := s.Get(ctx, "core.group", ref.ID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if p, err := uuid.Parse(fmt.Sprint(g["responsible"])); err == nil {
			out = append(out, Ref{Type: "core.person", ID: p})
		}
	}
	return out, nil
}

// holders are the persons and groups that hold a role of a record.
func holders(rec Record, role string) []Ref {
	all, _ := rec[actorsField].(map[string]any)
	list, _ := all[role].([]any)
	var out []Ref
	for _, h := range list {
		m, _ := h.(map[string]any)
		id, err := uuid.Parse(fmt.Sprint(m["id"]))
		typ := fmt.Sprint(m["type"])
		if err != nil || (typ != "core.person" && typ != "core.group") {
			continue
		}
		out = append(out, Ref{Type: typ, ID: id})
	}
	return out
}

// CheckApprovalTemplate is the RecordCheck of the stages of templates: the roles parse.
func CheckApprovalTemplate(_ context.Context, t meta.Type, rec map[string]any) []Problem {
	if t.QualifiedName() != TypeApprovalTemplateStage {
		return nil
	}
	text, _ := rec["roles"].(string)
	if _, err := ParseRoleSpecs(text); err != nil {
		return []Problem{{Field: "roles", Code: "invalid", Message: err.Error()}}
	}
	return nil
}

// templateOf reads the working days between reminders of a template a request names; a
// template that does not exist is a problem of the request.
func (s *Service) templateOf(ctx context.Context, req *ApprovalRequest) error {
	if req.Template == nil {
		return nil
	}
	tpl, err := s.Get(systemOf(ctx), TypeApprovalTemplate, *req.Template)
	if errors.Is(err, ErrNotFound) {
		return &ValidationError{Problems: []Problem{{Field: "template", Code: "not_found", Message: fmt.Sprintf("approval template %s does not exist", *req.Template)}}}
	}
	if err != nil {
		return err
	}
	if n, ok := tpl["remind_days"].(float64); ok && n >= 0 {
		days := int(n)
		req.remindDays = &days
	}
	return nil
}

// remindAt is when an open stage reminds first, or next after from: the working days of the
// approval (from its template), else the setting of the record's node, else the default; nil
// for no reminder. The working days count in the calendar of the record's node (WorkingDays).
func (s *Service) remindAt(ctx context.Context, rec Record, days *int, from time.Time) *time.Time {
	path, _ := rec[orgPathField].(string)
	n := DefaultRemindDays
	switch {
	case days != nil:
		n = *days
	default:
		var v float64
		if raw := s.setting(ctx, path, SettingRemindDays); raw != nil && json.Unmarshal(raw, &v) == nil && v >= 0 {
			n = int(v)
		}
	}
	if n <= 0 {
		return nil
	}
	at := from.AddDate(0, 0, n)
	if s.WorkingDays != nil {
		t, err := s.WorkingDays(systemOf(ctx), path, from, n)
		if err != nil {
			slog.WarnContext(ctx, "approval: reminder counted in calendar days", "path", path, "error", err)
		} else {
			at = t
		}
	}
	return &at
}

// systemOf is the access of the system in the session's tenant.
func systemOf(ctx context.Context) context.Context {
	return authz.WithAccess(ctx, authz.SystemIn(authz.From(ctx).CurrentTenant()))
}
