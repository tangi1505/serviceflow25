// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// --- approvals (WF6, WF-Plan E17; KF4; Inventar G43, G44) ---
//
// Whoever may change a record of a type with the trait approvals asks for approval: stages
// one after the other, in a stage persons and groups at once, with the least number of yes
// the stage needs (0: all). A group is asked once; the first of its members decides for it. A
// substitute decides for the person she stands in for, and for that person's groups. A stage
// is given when enough said yes, refused when that can no longer be: then the remaining
// questions are dropped, and with the last stage given or one refused the approval is over.
// The record keeps the outcome in the field approval; the transition the type names follows
// after the write, with the decider as actor and the rights of the system. Every decision is
// a row of the history (action approval) and an event.

// The events of approvals: a stage asks (approvers in the payload), someone decided, the
// approval is over (outcome in the payload).
const (
	EventApprovalRequest  = "approval_request"
	EventApprovalDecision = "approval_decision"
	EventApproval         = "approval"
)

// The states of an approval, a stage and a question.
const (
	approvalOpen      = "open"
	approvalApproved  = "approved"
	approvalRejected  = "rejected"
	approvalWithdrawn = "withdrawn"
	stepWaiting       = "waiting"
	stepDropped       = "dropped"
)

// Limits of a request.
const (
	MaxApprovalStages    = 20
	MaxApproversPerStage = 50
)

// Ref names a person or a group.
type Ref struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

// Approval is one request for approval of a record, with its stages.
type Approval struct {
	ID          uuid.UUID       `json:"id"`
	State       string          `json:"state"`
	Stage       int             `json:"stage"`
	Message     string          `json:"message"`
	Requester   *uuid.UUID      `json:"requester,omitempty"`
	RequestedAt time.Time       `json:"requested_at"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
	Stages      []ApprovalStage `json:"stages"`
}

// ApprovalStage is one stage: its questions and how many yes it needs (0: all).
type ApprovalStage struct {
	Position int            `json:"position"`
	Name     string         `json:"name"`
	Need     int            `json:"need"`
	State    string         `json:"state"`
	Steps    []ApprovalStep `json:"steps"`
}

// ApprovalStep is the question to one person or group. Mine says the session may answer it
// now; OnBehalf is the person a substitute decided for.
type ApprovalStep struct {
	ID        uuid.UUID  `json:"id"`
	Approver  Ref        `json:"approver"`
	State     string     `json:"state"`
	Comment   string     `json:"comment,omitempty"`
	DecidedBy *uuid.UUID `json:"decided_by,omitempty"`
	OnBehalf  *uuid.UUID `json:"on_behalf,omitempty"`
	AskedAt   *time.Time `json:"asked_at,omitempty"`
	DecidedAt *time.Time `json:"decided_at,omitempty"`
	Mine      bool       `json:"mine"`
}

// StageRequest is one stage of a request.
type StageRequest struct {
	Name      string `json:"name"`
	Need      int    `json:"need"`
	Approvers []Ref  `json:"approvers"`
}

// ApprovalRequest asks for approval: the stages in their order and a text to the approvers.
type ApprovalRequest struct {
	Message string         `json:"message"`
	Stages  []StageRequest `json:"stages"`
}

func (s *Service) approvalType(ctx context.Context, typeName string) (meta.Type, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return meta.Type{}, err
	}
	if !t.HasApprovals() {
		return meta.Type{}, fmt.Errorf("%s has no approvals (trait approvals): %w", typeName, ErrNotFound)
	}
	return t, nil
}

// Approvals lists the approvals of a record the session reads, newest first.
func (s *Service) Approvals(ctx context.Context, typeName string, id uuid.UUID) ([]Approval, error) {
	t, err := s.approvalType(ctx, typeName)
	if err != nil {
		return nil, err
	}
	if _, err := s.Get(ctx, typeName, id); err != nil {
		return nil, err
	}
	return s.approvalsOf(ctx, s.db.Tenant(ctx), t, id, uuid.Nil)
}

// approvalsOf reads the approvals of a record, or the one approval only.
func (s *Service) approvalsOf(ctx context.Context, db querier, t meta.Type, id, only uuid.UUID) ([]Approval, error) {
	rows, err := db.Query(ctx, `SELECT id, state, stage, message, requester, requested_at, finished_at FROM approval
WHERE type_name = $1 AND record_id = $2 AND ($3::uuid = '00000000-0000-0000-0000-000000000000' OR id = $3)
ORDER BY requested_at DESC, id DESC`, t.QualifiedName(), id, only)
	if err != nil {
		return nil, fmt.Errorf("records: approvals: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Approval, error) {
		var a Approval
		err := row.Scan(&a.ID, &a.State, &a.Stage, &a.Message, &a.Requester, &a.RequestedAt, &a.FinishedAt)
		return a, err
	})
	if err != nil {
		return nil, fmt.Errorf("records: approvals: %w", err)
	}
	if len(list) == 0 {
		return []Approval{}, nil
	}
	ids := make([]uuid.UUID, len(list))
	at := map[uuid.UUID]int{}
	for i, a := range list {
		ids[i] = a.ID
		at[a.ID] = i
		list[i].Stages = []ApprovalStage{}
	}
	rows, err = db.Query(ctx, `SELECT approval_id, position, name, need, state FROM approval_stage
WHERE approval_id = ANY($1::uuid[]) ORDER BY approval_id, position`, ids)
	if err != nil {
		return nil, fmt.Errorf("records: approval stages: %w", err)
	}
	type stageRow struct {
		approval uuid.UUID
		stage    ApprovalStage
	}
	stages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stageRow, error) {
		var r stageRow
		err := row.Scan(&r.approval, &r.stage.Position, &r.stage.Name, &r.stage.Need, &r.stage.State)
		r.stage.Steps = []ApprovalStep{}
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("records: approval stages: %w", err)
	}
	for _, r := range stages {
		i := at[r.approval]
		list[i].Stages = append(list[i].Stages, r.stage)
	}
	rows, err = db.Query(ctx, `SELECT approval_id, stage, id, approver_type, approver_id, state, comment, decided_by, on_behalf, asked_at, decided_at
FROM approval_step WHERE approval_id = ANY($1::uuid[]) ORDER BY approval_id, stage, position`, ids)
	if err != nil {
		return nil, fmt.Errorf("records: approval steps: %w", err)
	}
	type stepRow struct {
		approval uuid.UUID
		stage    int
		step     ApprovalStep
	}
	steps, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stepRow, error) {
		var r stepRow
		err := row.Scan(&r.approval, &r.stage, &r.step.ID, &r.step.Approver.Type, &r.step.Approver.ID, &r.step.State,
			&r.step.Comment, &r.step.DecidedBy, &r.step.OnBehalf, &r.step.AskedAt, &r.step.DecidedAt)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("records: approval steps: %w", err)
	}
	a := authz.From(ctx)
	now := time.Now()
	for _, r := range steps {
		ap := &list[at[r.approval]]
		if r.stage < 1 || r.stage > len(ap.Stages) {
			continue
		}
		st := &ap.Stages[r.stage-1]
		if r.step.State == approvalOpen {
			_, r.step.Mine = decides(a, now, r.step)
		}
		st.Steps = append(st.Steps, r.step)
	}
	return list, nil
}

// decides reports whether the access may answer an open question, and for whom: nil when it
// is her own or her group's, the person she stands in for else.
func decides(a *authz.Access, now time.Time, st ApprovalStep) (*uuid.UUID, bool) {
	if a == nil || a.PersonID == nil || a.IsSystem() {
		return nil, false
	}
	switch st.Approver.Type {
	case "core.person":
		if st.Approver.ID == *a.PersonID {
			return nil, true
		}
	case "core.group":
		for _, g := range a.MyGroups() {
			if g == st.Approver.ID {
				return nil, true
			}
		}
	}
	for _, r := range a.Standing(now) {
		person := r.Person
		if st.Approver.Type == "core.person" && st.Approver.ID == person {
			return &person, true
		}
		if st.Approver.Type == "core.group" {
			for _, g := range r.Groups {
				if g == st.Approver.ID {
					return &person, true
				}
			}
		}
	}
	return nil, false
}

// checkApprovalRequest checks a request: stages, names, the least numbers and approvers that
// exist, each once in a stage.
func (s *Service) checkApprovalRequest(ctx context.Context, req ApprovalRequest) error {
	var ps []Problem
	if len([]rune(req.Message)) > MaxEntryLength {
		ps = append(ps, Problem{Field: "message", Code: "max_length", Message: fmt.Sprintf("is longer than %d characters", MaxEntryLength)})
	}
	if len(req.Stages) == 0 || len(req.Stages) > MaxApprovalStages {
		ps = append(ps, Problem{Field: "stages", Code: "invalid", Message: fmt.Sprintf("needs 1 to %d stages", MaxApprovalStages)})
	}
	system := authz.WithAccess(ctx, authz.SystemIn(authz.From(ctx).CurrentTenant()))
	for i, st := range req.Stages {
		at := fmt.Sprintf("stages[%d]", i)
		if len([]rune(st.Name)) > 200 {
			ps = append(ps, Problem{Field: at + ".name", Code: "max_length", Message: "is longer than 200 characters"})
		}
		if len(st.Approvers) == 0 || len(st.Approvers) > MaxApproversPerStage {
			ps = append(ps, Problem{Field: at + ".approvers", Code: "invalid", Message: fmt.Sprintf("needs 1 to %d persons or groups", MaxApproversPerStage)})
		}
		if st.Need < 0 || st.Need > len(st.Approvers) {
			ps = append(ps, Problem{Field: at + ".need", Code: "invalid", Message: "must be 0 (all) up to the number of approvers"})
		}
		seen := map[Ref]bool{}
		for j, ref := range st.Approvers {
			field := fmt.Sprintf("%s.approvers[%d]", at, j)
			if ref.Type != "core.person" && ref.Type != "core.group" {
				ps = append(ps, Problem{Field: field, Code: "invalid", Message: "must be a core.person or a core.group"})
				continue
			}
			if seen[ref] {
				ps = append(ps, Problem{Field: field, Code: "duplicate", Message: "is asked twice in this stage"})
				continue
			}
			seen[ref] = true
			ok, err := s.exists(system, ref.Type, ref.ID)
			if err != nil {
				return err
			}
			if !ok {
				ps = append(ps, Problem{Field: field, Code: "not_found", Message: fmt.Sprintf("%s %s does not exist", ref.Type, ref.ID)})
			}
		}
	}
	if len(ps) > 0 {
		return &ValidationError{Problems: ps}
	}
	return nil
}

// RequestApproval asks for approval of a record the session may change. A record has one
// open approval at most; the transition "requested" of the type follows, and must be
// possible now.
func (s *Service) RequestApproval(ctx context.Context, typeName string, id uuid.UUID, req ApprovalRequest, actor Actor) (Approval, error) {
	t, err := s.approvalType(ctx, typeName)
	if err != nil {
		return Approval{}, err
	}
	if err := s.checkApprovalRequest(ctx, req); err != nil {
		return Approval{}, err
	}
	aid, err := uuid.NewV7()
	if err != nil {
		return Approval{}, err
	}
	var asked []Ref
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rec, err := s.read(ctx, tx, t, id, false, true)
		if err != nil {
			return err
		}
		if err := checkAccess(ctx, t, rec, authz.ActionUpdate); err != nil {
			return err
		}
		if err := s.followable(t, rec, approvalSpec(t).Requested); err != nil {
			return err
		}
		var open int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM approval WHERE type_name = $1 AND record_id = $2 AND state = 'open'`,
			t.QualifiedName(), id).Scan(&open); err != nil {
			return fmt.Errorf("records: approval: %w", err)
		}
		if open > 0 {
			return fmt.Errorf("%w: %s %s has an open approval; withdraw it first", ErrConflict, t.QualifiedName(), id)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO approval (id, type_name, record_id, state, stage, message, requested_by, requester)
VALUES ($1, $2, $3, 'open', 1, $4, $5, $6)`, aid, t.QualifiedName(), id, strings.TrimSpace(req.Message), nullActor(actor), authz.From(ctx).Me()); err != nil {
			return fmt.Errorf("records: approval: %w", err)
		}
		for i, st := range req.Stages {
			state := stepWaiting
			if i == 0 {
				state = approvalOpen
			}
			if _, err := tx.Exec(ctx, `INSERT INTO approval_stage (approval_id, position, name, need, state) VALUES ($1, $2, $3, $4, $5)`,
				aid, i+1, strings.TrimSpace(st.Name), st.Need, state); err != nil {
				return fmt.Errorf("records: approval stage: %w", err)
			}
			for j, ref := range st.Approvers {
				sid, err := uuid.NewV7()
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO approval_step (id, approval_id, stage, position, type_name, record_id, approver_type, approver_id, state, asked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, CASE WHEN $9 = 'open' THEN now() END)`,
					sid, aid, i+1, j+1, t.QualifiedName(), id, ref.Type, ref.ID, state); err != nil {
					return fmt.Errorf("records: approval step: %w", err)
				}
			}
		}
		asked = req.Stages[0].Approvers
		return s.approvalWritten(ctx, tx, t, id, rec, meta.ApprovalPending, EventApprovalRequest,
			map[string]any{"approval": aid.String(), "requested": true},
			map[string]any{"approval": aid.String(), "stage": 1, "approvers": asked}, actor)
	})
	if err = s.written(ctx, typeName, err); err != nil {
		return Approval{}, err
	}
	s.follow(ctx, t, id, approvalSpec(t).Requested, "", actor)
	return s.oneApproval(ctx, t, id, aid)
}

// Decide answers an open question of the current stage: the one named, or the first the
// session may answer (her own before her group's before one she stands in for). A refusal
// needs a reason.
func (s *Service) Decide(ctx context.Context, typeName string, id, approvalID uuid.UUID, stepID *uuid.UUID, approve bool, comment string, actor Actor) (Approval, error) {
	t, err := s.approvalType(ctx, typeName)
	if err != nil {
		return Approval{}, err
	}
	comment = strings.TrimSpace(comment)
	switch {
	case !approve && comment == "":
		return Approval{}, &ValidationError{Problems: []Problem{{Field: "comment", Code: "required", Message: "a refusal needs a reason"}}}
	case len([]rune(comment)) > MaxEntryLength:
		return Approval{}, &ValidationError{Problems: []Problem{{Field: "comment", Code: "max_length", Message: fmt.Sprintf("is longer than %d characters", MaxEntryLength)}}}
	}
	a := authz.From(ctx)
	if a.Me() == nil {
		return Approval{}, fmt.Errorf("%w: deciding needs a person linked to the account", ErrForbidden)
	}
	outcome := ""
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rec, err := s.read(ctx, tx, t, id, false, true)
		if err != nil {
			return err
		}
		list, err := s.approvalsOf(ctx, tx, t, id, approvalID)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return fmt.Errorf("approval %s of %s %s: %w", approvalID, t.QualifiedName(), id, ErrNotFound)
		}
		ap := list[0]
		if ap.State != approvalOpen {
			return fmt.Errorf("%w: the approval is %s", ErrConflict, ap.State)
		}
		stage := ap.Stages[ap.Stage-1]
		var pick *ApprovalStep
		var onBehalf *uuid.UUID
		rank := 3
		now := time.Now()
		for i := range stage.Steps {
			st := stage.Steps[i]
			if st.State != approvalOpen || (stepID != nil && st.ID != *stepID) {
				continue
			}
			who, ok := decides(a, now, st)
			if !ok {
				continue
			}
			r := 2
			switch {
			case who == nil && st.Approver.Type == "core.person":
				r = 0
			case who == nil:
				r = 1
			}
			if r < rank {
				rank, pick, onBehalf = r, &stage.Steps[i], who
			}
		}
		if pick == nil {
			if err := checkAccess(ctx, t, rec, authz.ActionRead); err != nil && !s.asked(ctx, tx, t, id) {
				return err
			}
			return fmt.Errorf("%w: you are not asked in the current stage of this approval", ErrForbidden)
		}
		decision := approvalApproved
		if !approve {
			decision = approvalRejected
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_step SET state = $2, comment = $3, decided_by = $4, on_behalf = $5, decided_at = now() WHERE id = $1`,
			pick.ID, decision, comment, a.Me(), onBehalf); err != nil {
			return fmt.Errorf("records: decide: %w", err)
		}
		pick.State = decision
		yes, open := 0, 0
		for _, st := range stage.Steps {
			switch st.State {
			case approvalApproved:
				yes++
			case approvalOpen:
				open++
			}
		}
		need := stage.Need
		if need == 0 || need > len(stage.Steps) {
			need = len(stage.Steps)
		}
		q := tx
		stageState := ""
		switch {
		case yes >= need:
			stageState = approvalApproved
		case yes+open < need:
			stageState = approvalRejected
		}
		var next []Ref
		if stageState != "" {
			if _, err := q.Exec(ctx, `UPDATE approval_stage SET state = $3 WHERE approval_id = $1 AND position = $2`, ap.ID, ap.Stage, stageState); err != nil {
				return fmt.Errorf("records: decide: %w", err)
			}
			if _, err := q.Exec(ctx, `UPDATE approval_step SET state = 'dropped' WHERE approval_id = $1 AND stage = $2 AND state = 'open'`, ap.ID, ap.Stage); err != nil {
				return fmt.Errorf("records: decide: %w", err)
			}
			switch {
			case stageState == approvalRejected:
				outcome = approvalRejected
			case ap.Stage == len(ap.Stages):
				outcome = approvalApproved
			default:
				following := ap.Stages[ap.Stage]
				for _, st := range following.Steps {
					next = append(next, st.Approver)
				}
				if _, err := q.Exec(ctx, `UPDATE approval SET stage = $2 WHERE id = $1`, ap.ID, ap.Stage+1); err != nil {
					return fmt.Errorf("records: decide: %w", err)
				}
				if _, err := q.Exec(ctx, `UPDATE approval_stage SET state = 'open' WHERE approval_id = $1 AND position = $2`, ap.ID, ap.Stage+1); err != nil {
					return fmt.Errorf("records: decide: %w", err)
				}
				if _, err := q.Exec(ctx, `UPDATE approval_step SET state = 'open', asked_at = now() WHERE approval_id = $1 AND stage = $2`, ap.ID, ap.Stage+1); err != nil {
					return fmt.Errorf("records: decide: %w", err)
				}
			}
		}
		if outcome != "" {
			if _, err := q.Exec(ctx, `UPDATE approval SET state = $2, finished_at = now() WHERE id = $1`, ap.ID, outcome); err != nil {
				return fmt.Errorf("records: decide: %w", err)
			}
			if _, err := q.Exec(ctx, `UPDATE approval_stage SET state = 'dropped' WHERE approval_id = $1 AND state = 'waiting'`, ap.ID); err != nil {
				return fmt.Errorf("records: decide: %w", err)
			}
			if _, err := q.Exec(ctx, `UPDATE approval_step SET state = 'dropped' WHERE approval_id = $1 AND state IN ('waiting', 'open')`, ap.ID); err != nil {
				return fmt.Errorf("records: decide: %w", err)
			}
		}
		entry := map[string]any{"approval": ap.ID.String(), "stage": ap.Stage, "step": pick.ID.String(),
			"approver": pick.Approver, "decision": decision}
		if comment != "" {
			entry["comment"] = comment
		}
		if onBehalf != nil {
			entry["on_behalf"] = onBehalf.String()
		}
		field := ""
		switch outcome {
		case approvalApproved:
			field = meta.ApprovalApproved
		case approvalRejected:
			field = meta.ApprovalRejected
		}
		kind, payload := EventApprovalDecision, map[string]any{"approval": ap.ID.String(), "stage": ap.Stage, "decision": decision}
		switch {
		case outcome != "":
			kind, payload = EventApproval, map[string]any{"approval": ap.ID.String(), "outcome": outcome}
		case len(next) > 0:
			kind, payload = EventApprovalRequest, map[string]any{"approval": ap.ID.String(), "stage": ap.Stage + 1, "approvers": next}
		}
		return s.approvalWritten(ctx, tx, t, id, rec, field, kind, entry, payload, actor)
	})
	if err = s.written(ctx, typeName, err); err != nil {
		return Approval{}, err
	}
	switch outcome {
	case approvalApproved:
		s.follow(ctx, t, id, approvalSpec(t).Approved, "", actor)
	case approvalRejected:
		s.follow(ctx, t, id, approvalSpec(t).Rejected, comment, actor)
	}
	return s.oneApproval(ctx, t, id, approvalID)
}

// WithdrawApproval ends an open approval of a record the session may change: every question
// still open or waiting is dropped. No transition follows.
func (s *Service) WithdrawApproval(ctx context.Context, typeName string, id, approvalID uuid.UUID, actor Actor) (Approval, error) {
	t, err := s.approvalType(ctx, typeName)
	if err != nil {
		return Approval{}, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rec, err := s.read(ctx, tx, t, id, false, true)
		if err != nil {
			return err
		}
		if err := checkAccess(ctx, t, rec, authz.ActionUpdate); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE approval SET state = 'withdrawn', finished_at = now()
WHERE id = $1 AND type_name = $2 AND record_id = $3 AND state = 'open'`, approvalID, t.QualifiedName(), id)
		if err != nil {
			return fmt.Errorf("records: withdraw: %w", err)
		}
		if tag.RowsAffected() != 1 {
			var state string
			err := tx.QueryRow(ctx, `SELECT state FROM approval WHERE id = $1 AND type_name = $2 AND record_id = $3`, approvalID, t.QualifiedName(), id).Scan(&state)
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("approval %s of %s %s: %w", approvalID, t.QualifiedName(), id, ErrNotFound)
			}
			return fmt.Errorf("%w: the approval is %s", ErrConflict, state)
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_stage SET state = 'dropped' WHERE approval_id = $1 AND state IN ('waiting', 'open')`, approvalID); err != nil {
			return fmt.Errorf("records: withdraw: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_step SET state = 'dropped' WHERE approval_id = $1 AND state IN ('waiting', 'open')`, approvalID); err != nil {
			return fmt.Errorf("records: withdraw: %w", err)
		}
		return s.approvalWritten(ctx, tx, t, id, rec, meta.ApprovalWithdrawn, EventApproval,
			map[string]any{"approval": approvalID.String(), "withdrawn": true},
			map[string]any{"approval": approvalID.String(), "outcome": approvalWithdrawn}, actor)
	})
	if err = s.written(ctx, typeName, err); err != nil {
		return Approval{}, err
	}
	return s.oneApproval(ctx, t, id, approvalID)
}

func (s *Service) oneApproval(ctx context.Context, t meta.Type, id, approvalID uuid.UUID) (Approval, error) {
	list, err := s.approvalsOf(ctx, s.db.Tenant(ctx), t, id, approvalID)
	if err != nil {
		return Approval{}, err
	}
	if len(list) == 0 {
		return Approval{}, fmt.Errorf("approval %s: %w", approvalID, ErrNotFound)
	}
	return list[0], nil
}

// approvalWritten sets the field approval when it changes (empty: it stays), and leaves the
// history row of the step and the event. The record keeps its version, as with costs.
func (s *Service) approvalWritten(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, rec Record, field, kind string, entry, payload map[string]any, actor Actor) error {
	changed := []string{}
	if field != "" {
		sql := fmt.Sprintf(`UPDATE %s SET %s = $2 WHERE id = $1`, table(t), pgx.Identifier{meta.FieldApproval}.Sanitize())
		if _, err := tx.Exec(ctx, sql, id, field); err != nil {
			return fmt.Errorf("records: approval field: %w", err)
		}
		changed = append(changed, meta.FieldApproval)
		entry["field"] = field
	}
	q := queries.New(tx)
	version := rec["version"].(int64)
	if err := audit(ctx, q, t, id, version, EventApproval, &change{meta.FieldApproval, rec[meta.FieldApproval], entry}, actor); err != nil {
		return err
	}
	return event(ctx, q, t, id, rec, kind, version, changed, payload, actor)
}

// approvalSpec is the block approval of the type, empty without one.
func approvalSpec(t meta.Type) meta.ApprovalSpec {
	if t.Approval == nil {
		return meta.ApprovalSpec{}
	}
	return *t.Approval
}

// followable refuses a request whose transition "requested" is not possible now.
func (s *Service) followable(t meta.Type, rec Record, name string) error {
	if name == "" {
		return nil
	}
	tr, ok := t.Transition(name)
	if !ok || tr.Disabled {
		return nil
	}
	return moveAllowed(tr, rec)
}

// follow takes the transition after an approval with the rights of the system and the decider
// as actor; the reason of a refusal goes along as its text when it takes one. When the record
// cannot move (another state meanwhile, a required field), the history says so and the
// approval stands.
func (s *Service) follow(ctx context.Context, t meta.Type, id uuid.UUID, name, comment string, actor Actor) {
	if name == "" {
		return
	}
	tr, ok := t.Transition(name)
	if !ok {
		return
	}
	system := authz.WithAccess(ctx, authz.SystemIn(authz.From(ctx).CurrentTenant()))
	if comment != "" && tr.Comment != "" {
		system = WithComment(system, comment)
	}
	if _, err := s.Transition(system, t.QualifiedName(), id, name, nil, -1, actor); err != nil {
		slog.WarnContext(ctx, "approval: transition did not follow", "type", t.QualifiedName(), "id", id, "transition", name, "error", err)
		_ = s.Trace(system, t.QualifiedName(), id, EventApproval, map[string]any{"transition": name, "failed": err.Error()}, actor)
	}
}

// asked reports whether the session is asked, or was and answered, at an approval of the
// record: then she reads it without a grant (WF6).
func (s *Service) asked(ctx context.Context, db querier, t meta.Type, id uuid.UUID) bool {
	a := authz.From(ctx)
	if a.IsSystem() || !t.HasApprovals() {
		return false
	}
	ids := a.Asked(time.Now())
	if len(ids) == 0 {
		return false
	}
	rows, err := db.Query(ctx, `SELECT 1 FROM approval_step WHERE type_name = $1 AND record_id = $2
AND state IN ('open', 'approved', 'rejected') AND approver_id = ANY($3::uuid[]) LIMIT 1`, t.QualifiedName(), id, ids)
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

// MyApproval is one question in the list "Zu genehmigen": open ones the session may answer,
// or answered ones she decided.
type MyApproval struct {
	Approval    uuid.UUID  `json:"approval"`
	Step        uuid.UUID  `json:"step"`
	TypeName    string     `json:"type_name"`
	RecordID    uuid.UUID  `json:"record_id"`
	Title       string     `json:"title"`
	Stage       int        `json:"stage"`
	Stages      int        `json:"stages"`
	StageName   string     `json:"stage_name"`
	Approver    Ref        `json:"approver"`
	State       string     `json:"state"`
	OnBehalf    *uuid.UUID `json:"on_behalf,omitempty"`
	Requester   *uuid.UUID `json:"requester,omitempty"`
	RequestedAt time.Time  `json:"requested_at"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
}

// MyApprovals lists the open questions the session may answer (open) or those she answered,
// newest first; limit bounds the list.
func (s *Service) MyApprovals(ctx context.Context, open bool, limit int) ([]MyApproval, error) {
	a := authz.From(ctx)
	out := []MyApproval{}
	if a.IsSystem() || a.Me() == nil {
		return out, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	now := time.Now()
	var rows pgx.Rows
	var err error
	const cols = `SELECT st.approval_id, st.id, st.type_name, st.record_id, st.stage, (SELECT count(*) FROM approval_stage x WHERE x.approval_id = ap.id),
  sg.name, st.approver_type, st.approver_id, st.state, st.on_behalf, ap.requester, ap.requested_at, st.decided_at
FROM approval_step st JOIN approval ap ON ap.id = st.approval_id JOIN approval_stage sg ON sg.approval_id = st.approval_id AND sg.position = st.stage`
	if open {
		rows, err = s.db.Tenant(ctx).Query(ctx, cols+` WHERE st.state = 'open' AND st.approver_id = ANY($1::uuid[]) ORDER BY ap.requested_at DESC LIMIT $2`,
			a.Asked(now), limit)
	} else {
		rows, err = s.db.Tenant(ctx).Query(ctx, cols+` WHERE st.state IN ('approved', 'rejected') AND st.decided_by = $1 ORDER BY st.decided_at DESC LIMIT $2`,
			*a.Me(), limit)
	}
	if err != nil {
		return nil, fmt.Errorf("records: my approvals: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MyApproval, error) {
		var m MyApproval
		var stages int64
		err := row.Scan(&m.Approval, &m.Step, &m.TypeName, &m.RecordID, &m.Stage, &stages, &m.StageName, &m.Approver.Type, &m.Approver.ID,
			&m.State, &m.OnBehalf, &m.Requester, &m.RequestedAt, &m.DecidedAt)
		m.Stages = int(stages)
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("records: my approvals: %w", err)
	}
	for _, m := range list {
		if open {
			who, ok := decides(a, now, ApprovalStep{Approver: m.Approver, State: approvalOpen})
			if !ok {
				continue
			}
			m.OnBehalf = who
		}
		t, err := s.Type(ctx, m.TypeName)
		if err != nil {
			continue
		}
		rec, err := s.Get(ctx, m.TypeName, m.RecordID)
		if err != nil {
			// Trashed meanwhile, or another tenant: not on the list.
			continue
		}
		if t.TitleField != "" {
			m.Title, _ = rec[t.TitleField].(string)
		}
		out = append(out, m)
	}
	return out, nil
}
