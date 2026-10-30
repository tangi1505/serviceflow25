// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/rules/engine"
)

// --- rules "if, then" (WF4) ---
//
// Reading needs "read" on the type "core.rule", changing "manage_rules" on it (admin has both
// through * *): a rule acts with the rights of the system, so whoever writes one needs more
// than the right on the type it reacts to.

const rulesType = "core.rule"

func (s *server) rulesReady(w http.ResponseWriter, r *http.Request, action string) bool {
	if s.o.Rules == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "rules are not configured on this instance", nil)
		return false
	}
	if a := authz.From(r.Context()); !a.Allowed(rulesType, action) {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "rules need the right "+action+" on core.rule", nil)
		return false
	}
	return true
}

// mayChangeInstallationRule answers 403 when a rule of the installation is to be changed from
// a tenant without manage_rules at the root: such a rule acts in every tenant (PL45).
func (s *server) mayChangeInstallationRule(w http.ResponseWriter, r *http.Request, installation bool) bool {
	if a := authz.From(r.Context()); installation && a.Bounded() && !a.Installation(rulesType, "manage_rules") {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "a rule of the installation acts in every tenant: it needs manage_rules on core.rule at the root", nil)
		return false
	}
	return true
}

func (s *server) ListRules(w http.ResponseWriter, r *http.Request) {
	if !s.rulesReady(w, r, authz.ActionRead) {
		return
	}
	list, err := s.o.Rules.List(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) GetRule(w http.ResponseWriter, r *http.Request, name gen.RuleName) {
	if !s.rulesReady(w, r, authz.ActionRead) {
		return
	}
	rule, err := s.o.Rules.Get(r.Context(), name)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// rulePut is the body of PUT: the rule without what the server keeps.
type rulePut struct {
	Label       meta.Label      `json:"label"`
	Description meta.Label      `json:"description"`
	TypeName    string          `json:"type_name"`
	Trigger     string          `json:"trigger"`
	Mode        string          `json:"mode"`
	Condition   string          `json:"condition"`
	Schedule    string          `json:"schedule"`
	SLA         *uuid.UUID      `json:"sla"`
	Percent     *int            `json:"percent"`
	Minutes     *int            `json:"minutes"`
	Actions     []engine.Action `json:"actions"`
	Position    int             `json:"position"`
	Stop        bool            `json:"stop"`
	Enabled     *bool           `json:"enabled"`
	// Installation makes a new rule one of the installation, acting in every tenant (PL45).
	Installation bool `json:"installation"`
}

func (s *server) PutRule(w http.ResponseWriter, r *http.Request, name gen.RuleName) {
	if !s.rulesReady(w, r, "manage_rules") {
		return
	}
	var in rulePut
	if !decode(w, r, &in) {
		return
	}
	installation := in.Installation
	if existing, err := s.o.Rules.Get(r.Context(), name); err == nil && existing.Tenant == "" {
		installation = true
	}
	if !s.mayChangeInstallationRule(w, r, installation) {
		return
	}
	enabled := in.Enabled == nil || *in.Enabled
	rule, created, err := s.o.Rules.Put(r.Context(), name, engine.Rule{Label: in.Label, Description: in.Description,
		TypeName: in.TypeName, Trigger: in.Trigger, Mode: in.Mode, Condition: in.Condition, Schedule: in.Schedule,
		SLA: in.SLA, Percent: in.Percent, Minutes: in.Minutes, Actions: in.Actions, Position: in.Position, Stop: in.Stop, Enabled: enabled},
		installation, (*uuid.UUID)(actor(r)))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, rule)
}

func (s *server) DeleteRule(w http.ResponseWriter, r *http.Request, name gen.RuleName) {
	if !s.rulesReady(w, r, "manage_rules") {
		return
	}
	if existing, err := s.o.Rules.Get(r.Context(), name); err == nil && !s.mayChangeInstallationRule(w, r, existing.Tenant == "") {
		return
	}
	s.noContent(w, r, s.o.Rules.Delete(r.Context(), name))
}
