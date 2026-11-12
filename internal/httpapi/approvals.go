// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/records"
)

// --- approvals (WF6, WF-Plan E17) ---

func (s *server) RecordApprovals(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	list, err := s.o.Records.Approvals(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) RequestApproval(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	var body records.ApprovalRequest
	if !decode(w, r, &body) {
		return
	}
	a, err := s.o.Records.RequestApproval(r.Context(), typeName, id, body, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// ApprovalTemplates lists the templates of approvals filled in for the record (WF7).
func (s *server) ApprovalTemplates(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	list, err := s.o.Records.ApprovalTemplates(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) DecideApproval(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, approvalID gen.ApprovalId) {
	var body struct {
		Decision string     `json:"decision"`
		Comment  string     `json:"comment"`
		Step     *uuid.UUID `json:"step"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Decision != "approve" && body.Decision != "reject" {
		writeError(w, r, s.o.Logger, &records.ValidationError{Problems: []records.Problem{{Field: "decision", Code: "invalid", Message: "must be approve or reject"}}})
		return
	}
	a, err := s.o.Records.Decide(r.Context(), typeName, id, approvalID, body.Step, body.Decision == "approve", body.Comment, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *server) WithdrawApproval(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, approvalID gen.ApprovalId) {
	a, err := s.o.Records.WithdrawApproval(r.Context(), typeName, id, approvalID, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *server) MyApprovals(w http.ResponseWriter, r *http.Request, p gen.MyApprovalsParams) {
	open := p.State == nil || *p.State == gen.MyApprovalsParamsStateOpen
	limit := 0
	if p.Limit != nil {
		limit = *p.Limit
	}
	list, err := s.o.Records.MyApprovals(r.Context(), open, limit)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// --- substitutes (WF6) ---

func (s *server) PersonSubstitutes(w http.ResponseWriter, r *http.Request, personID gen.PersonId) {
	list, err := s.o.Records.Substitutes(r.Context(), personID)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) AddSubstitute(w http.ResponseWriter, r *http.Request, personID gen.PersonId) {
	var body struct {
		Substitute uuid.UUID  `json:"substitute"`
		From       *time.Time `json:"valid_from"`
		Until      *time.Time `json:"valid_until"`
	}
	if !decode(w, r, &body) {
		return
	}
	x, err := s.o.Records.AddSubstitute(r.Context(), personID, body.Substitute, body.From, body.Until, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.substitutesChanged(r)
	writeJSON(w, http.StatusCreated, x)
}

func (s *server) RemoveSubstitute(w http.ResponseWriter, r *http.Request, personID gen.PersonId, substituteID openapi_types.UUID) {
	if err := s.o.Records.RemoveSubstitute(r.Context(), personID, substituteID); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.substitutesChanged(r)
	w.WriteHeader(http.StatusNoContent)
}

// substitutesChanged lets every instance load the rights anew: a substitute reads what the
// person she stands in for is asked (WF6).
func (s *server) substitutesChanged(r *http.Request) {
	if s.o.Authz != nil {
		s.o.Authz.Invalidate(r.Context())
	}
}
