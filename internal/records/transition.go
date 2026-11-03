// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/rules"
)

// ErrNotAllowed says a transition does not apply to the record as it is (wrong state or
// condition false); the message carries the reason. It answers as 409 (WF1).
var ErrNotAllowed = errors.New("transition not allowed")

// TransitionInfo tells whether a transition applies to a record right now.
type TransitionInfo struct {
	Name    string     `json:"name"`
	Label   meta.Label `json:"label"`
	To      string     `json:"to"`
	Allowed bool       `json:"allowed"`
	// Reason says why it is not allowed: "state", "condition" or "role" (SD-Plan N8).
	Reason string `json:"reason,omitempty"`
	// By and Comment come from the definition: who may take it, whether it takes a text.
	By      []string `json:"by,omitempty"`
	Comment string   `json:"comment,omitempty"`
}

// Event is one row of the outbox (E39) as the API shows it.
type Event struct {
	Position int64           `json:"position"`
	At       string          `json:"at"`
	Kind     string          `json:"kind"`
	TypeName string          `json:"type_name"`
	RecordID uuid.UUID       `json:"record_id"`
	Actor    *uuid.UUID      `json:"actor,omitempty"`
	Payload  json.RawMessage `json:"payload"`
	// Mine marks, in the stream of a session, an event of a record where the person or one
	// of their groups holds a role (UI8: notifications in the browser).
	Mine bool `json:"mine,omitempty"`
}

// Transitions lists every transition of the type with whether it applies to the record now.
func (s *Service) Transitions(ctx context.Context, typeName string, id uuid.UUID) ([]TransitionInfo, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	rec, err := s.Get(ctx, typeName, id)
	if err != nil {
		return nil, err
	}
	out := make([]TransitionInfo, 0, len(t.Transitions))
	for _, tr := range t.Transitions {
		// A transition the customer layer disabled (UI26.1) is not offered.
		if tr.Disabled {
			continue
		}
		info := TransitionInfo{Name: tr.Name, Label: tr.Label, To: tr.To, By: tr.By, Comment: tr.Comment}
		info.Allowed, info.Reason, err = applies(tr, rec)
		if err != nil {
			return nil, err
		}
		// A transition of a role is the person's only if they hold it (SD-Plan N8).
		if info.Allowed && len(tr.By) > 0 && !authz.From(ctx).HoldsAnyRole(rec, tr.By) {
			info.Allowed, info.Reason = false, "role"
		}
		out = append(out, info)
	}
	return out, nil
}

// applies checks state and condition of a transition against a record.
func applies(tr meta.Transition, rec Record) (bool, string, error) {
	state, _ := rec["state"].(string)
	if !tr.Allows(state) {
		return false, "state", nil
	}
	if tr.When == "" {
		return true, "", nil
	}
	cond, err := rules.Compile(tr.When)
	if err != nil {
		return false, "", err
	}
	ok, err := cond.Eval(map[string]any(rec))
	if err != nil {
		return false, "", err
	}
	if !ok {
		return false, "condition", nil
	}
	return true, "", nil
}

// moveAllowed checks a transition against the record as it will be: ErrNotAllowed with the
// reason when it does not start from the record's state or its condition is not met.
func moveAllowed(tr meta.Transition, rec map[string]any) error {
	allowed, reason, err := applies(tr, rec)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}
	state, _ := rec[stateField].(string)
	if reason == "state" {
		return fmt.Errorf("%w: %q does not start from state %q", ErrNotAllowed, tr.Name, state)
	}
	return fmt.Errorf("%w: the condition of %q is not met (%s)", ErrNotAllowed, tr.Name, tr.When)
}

// Transition moves the record along the named transition, with fields set in the same
// version (WF2): the row is locked, state and condition are checked against the record as it
// will be, the field states of the target state must hold, and the fields, the state, the
// audit rows and one event of kind transition are written in the same transaction (WF1, E39).
// expectedVersion below 0 takes the version the record has.
func (s *Service) Transition(ctx context.Context, typeName string, id uuid.UUID, name string, fields map[string]any, expectedVersion int64, actor Actor) (Record, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	if !t.HasStates() {
		return nil, &ValidationError{Problems: []Problem{{Field: "transition", Code: "unknown", Message: t.QualifiedName() + " has no state model"}}}
	}
	tr, ok := t.Transition(name)
	if !ok {
		return nil, &ValidationError{Problems: []Problem{{Field: "transition", Code: "unknown", Message: fmt.Sprintf("%q is not a transition of %s", name, t.QualifiedName())}}}
	}
	if tr.Disabled {
		return nil, &ValidationError{Problems: []Problem{{Field: "transition", Code: "disabled", Message: fmt.Sprintf("%q is disabled by the customer layer", name)}}}
	}
	// A text with the transition becomes an entry in the same write (SD-Plan N8).
	comment, _ := ctx.Value(commentKey{}).(string)
	switch {
	case tr.Comment == meta.CommentRequired && strings.TrimSpace(comment) == "":
		return nil, &ValidationError{Problems: []Problem{{Field: "comment", Code: "required", Message: fmt.Sprintf("%q needs a comment", name)}}}
	case comment != "" && tr.Comment == "":
		return nil, &ValidationError{Problems: []Problem{{Field: "comment", Code: "unknown", Message: fmt.Sprintf("%q takes no comment", name)}}}
	case len([]rune(comment)) > MaxEntryLength:
		return nil, &ValidationError{Problems: []Problem{{Field: "comment", Code: "max_length", Message: fmt.Sprintf("is longer than %d characters", MaxEntryLength)}}}
	}
	if len(tr.By) > 0 {
		// Only a role at the record may take it, and needs no other right on the type: the
		// requester confirming a solution in the portal (SD-Plan N8).
		if len(fields) > 0 {
			return nil, &ValidationError{Problems: []Problem{{Field: "fields", Code: "unknown", Message: fmt.Sprintf("%q is taken by a role and sets no fields", name)}}}
		}
		a := authz.From(ctx)
		system := authz.WithAccess(ctx, authz.SystemIn(a.CurrentTenant()))
		rec, err := s.Get(system, typeName, id)
		if err != nil {
			return nil, err
		}
		if a.IsSystem() || !a.HoldsAnyRole(rec, tr.By) {
			return nil, fmt.Errorf("%w: only %s at the record may take %q", ErrForbidden, strings.Join(tr.By, " or "), name)
		}
		ctx = system
	}
	return s.update(ctx, t, id, expectedVersion, fields, actor, nil, &tr)
}

type commentKey struct{}

// WithComment hands the text a transition takes along (SD-Plan N8); Transition keeps it as an
// entry at the record in the same write.
func WithComment(ctx context.Context, body string) context.Context {
	return context.WithValue(ctx, commentKey{}, body)
}

// Events returns the outbox after a position, oldest first (the stream of E39; PL24 adds
// retention, SSE and the dispatcher).
func (s *Service) Events(ctx context.Context, after int64, limit int) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := queries.New(s.db.Tenant(ctx)).ListEvents(ctx, queries.ListEventsParams{Position: after, Limit: int32(limit)}) //nolint:gosec // limit is at most 1000
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		e := Event{Position: r.Position, At: r.At.Time.UTC().Format("2006-01-02T15:04:05.000000Z07:00"), Kind: r.Kind,
			TypeName: r.TypeName, RecordID: r.RecordID, Payload: r.Payload}
		if r.Actor.Valid {
			a := r.Actor.UUID
			e.Actor = &a
		}
		out = append(out, e)
	}
	return out, nil
}

// InState reports whether a record of the type stands in the state, in the database of the
// context, the trash included: the customer layer does not remove such a state (UI26.1).
func (s *Service) InState(ctx context.Context, typeName, state string) (bool, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return false, err
	}
	var found bool
	err = s.db.Tenant(ctx).QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+table(t)+" WHERE state = $1)", state).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("records: state %s of %s: %w", state, typeName, err)
	}
	return found, nil
}
