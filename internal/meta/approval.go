// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

// --- approvals (WF6, WF-Plan E17; KF4; Inventar G43, G44) ---
//
// A type with the trait approvals can ask for approval in stages: one after the other, in a
// stage everyone at once, with the least number of yes the stage needs. The core keeps the
// approvals in its own tables and the outcome in the computed field approval. The type says
// under "approval" which transition runs when an approval is asked, given or refused; the
// record moves on its own, with the decider as actor.

// FieldApproval is the computed field of the trait approvals: the state of the last
// approval of the record, empty before the first.
const FieldApproval = "approval"

// The values of the field approval.
const (
	ApprovalPending   = "pending"
	ApprovalApproved  = "approved"
	ApprovalRejected  = "rejected"
	ApprovalWithdrawn = "withdrawn"
)

// approvalField is the field the trait gives a type.
var approvalField = Field{Name: FieldApproval, Type: FieldChoice, Computed: true,
	Label: Label{"de": "Genehmigung", "en": "Approval"},
	Options: []Option{
		{Value: ApprovalPending, Label: Label{"de": "Offen", "en": "Pending"}},
		{Value: ApprovalApproved, Label: Label{"de": "Erteilt", "en": "Approved"}},
		{Value: ApprovalRejected, Label: Label{"de": "Abgelehnt", "en": "Rejected"}},
		{Value: ApprovalWithdrawn, Label: Label{"de": "Zurückgezogen", "en": "Withdrawn"}},
	}}

// ApprovalSpec names the transitions that follow an approval (WF6, your decision of
// 2026-11-10): Requested when it is asked, Approved when every stage agreed, Rejected when a
// stage can no longer agree. Each is optional; without one the record stays where it is and
// only the field approval tells.
type ApprovalSpec struct {
	Requested string `json:"requested,omitempty" yaml:"requested"`
	Approved  string `json:"approved,omitempty" yaml:"approved"`
	Rejected  string `json:"rejected,omitempty" yaml:"rejected"`
}

// HasApprovals reports whether records of the type can be approved.
func (t Type) HasApprovals() bool { return t.HasTrait(TraitApprovals) }

// validateApproval checks the block approval: only with the trait, and every name a
// transition of the type.
func validateApproval(add func(string, string, ...any), where string, t Type) {
	a := t.Approval
	if a == nil {
		return
	}
	w := where + ", approval"
	if !t.HasApprovals() {
		add(w, "needs the trait %s", TraitApprovals)
	}
	for _, x := range []struct{ key, name string }{{"requested", a.Requested}, {"approved", a.Approved}, {"rejected", a.Rejected}} {
		if x.name == "" {
			continue
		}
		if _, ok := t.Transition(x.name); !ok {
			add(w, "%s %q is not a transition of this type", x.key, x.name)
		}
	}
}
