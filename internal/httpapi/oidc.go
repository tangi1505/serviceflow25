// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
)

// --- sign-in through the identity provider (PL11) ---
//
// start and callback are public (E78): the browser is sent to the provider and comes back
// with a code; the session cookie is set and the browser sent on. Errors go back to the
// sign-in page as ?error=…, never as JSON, because the browser is in a redirect.

// Provider is one way to sign in, for the sign-in page.
type Provider struct {
	Name     string `json:"name"`
	StartURL string `json:"start_url"`
}

func (s *server) ListAuthProviders(w http.ResponseWriter, _ *http.Request) {
	list := []Provider{}
	if s.o.Authn.Enabled() {
		list = append(list, Provider{Name: s.o.Authn.Name(), StartURL: "/api/v1/auth/oidc/start"})
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) StartOIDC(w http.ResponseWriter, r *http.Request, p gen.StartOIDCParams) {
	if !s.o.Authn.Enabled() {
		writeProblem(w, r, http.StatusNotFound, "not_found", "no identity provider is configured", nil)
		return
	}
	next := "/"
	if p.Next != nil {
		next = *p.Next
	}
	target, err := s.o.Authn.Begin(r.Context(), next)
	if err != nil {
		s.o.Logger.Error("sign-in through the provider cannot start", "error", err.Error())
		http.Redirect(w, r, "/login?error=provider", http.StatusFound)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *server) CompleteOIDC(w http.ResponseWriter, r *http.Request, p gen.CompleteOIDCParams) {
	if p.Error != nil && *p.Error != "" {
		s.o.Logger.Info("provider refused the sign-in", "error", *p.Error)
		s.signIn(r, "login_oidc", nil, "", "refused_by_provider", map[string]any{"error": *p.Error})
		http.Redirect(w, r, "/login?error=refused", http.StatusFound)
		return
	}
	state, code := "", ""
	if p.State != nil {
		state = *p.State
	}
	if p.Code != nil {
		code = *p.Code
	}
	sess, next, err := s.o.Authn.Complete(r.Context(), state, code)
	if err != nil {
		reason := "sso"
		switch {
		case errors.Is(err, authn.ErrState):
			reason = "state"
		case errors.Is(err, authn.ErrRefused):
			reason = "disabled"
		}
		s.o.Logger.Warn("sign-in through the provider failed", "reason", reason, "error", err.Error())
		s.signIn(r, "login_oidc", nil, "", reason, nil)
		http.Redirect(w, r, "/login?error="+url.QueryEscape(reason), http.StatusFound)
		return
	}
	s.signIn(r, "login_oidc", &sess.AccountID, sess.Username, "", nil)
	setSessionCookie(w, r, sess)
	http.Redirect(w, r, next, http.StatusFound)
}

// --- login rules (G121) ---

func (s *server) ListLoginRules(w http.ResponseWriter, r *http.Request) {
	if s.o.Authn == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "login rules are not configured on this instance", nil)
		return
	}
	if !s.mayManageRights(w, r) {
		return
	}
	list, err := s.o.Authn.Rules(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	a := authz.From(r.Context())
	mine := []authn.Rule{}
	for _, rule := range list {
		if assignable(a, rule.OrgPath) {
			mine = append(mine, rule)
		}
	}
	writeJSON(w, http.StatusOK, mine)
}

// loginRuleNode answers 404 for a node outside the session's tenant (PL45): a login rule
// assigns roles there. An empty node is the tenant itself.
func (s *server) loginRuleNode(w http.ResponseWriter, r *http.Request, in *authn.RuleInput) bool {
	a := authz.From(r.Context())
	if in.OrgPath == "" {
		in.OrgPath = a.HomeNode()
	}
	if !assignable(a, in.OrgPath) {
		writeError(w, r, s.o.Logger, fmt.Errorf("%w: node %q", authz.ErrNotFound, in.OrgPath))
		return false
	}
	return true
}

// loginRuleInTenant answers 404 for a login rule of another tenant.
func (s *server) loginRuleInTenant(w http.ResponseWriter, r *http.Request, id uuid.UUID) bool {
	rule, err := s.o.Authn.Rule(r.Context(), id)
	if err == nil && !assignable(authz.From(r.Context()), rule.OrgPath) {
		err = fmt.Errorf("%w: login rule %s", authz.ErrNotFound, id)
	}
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return false
	}
	return true
}

func ruleInput(body gen.LoginRuleInput) authn.RuleInput {
	in := authn.RuleInput{Name: body.Name, Condition: body.Condition, Role: body.Role, Enabled: body.Enabled}
	if body.OrgPath != nil {
		in.OrgPath = *body.OrgPath
	}
	if body.Position != nil {
		p := int32(*body.Position) //nolint:gosec // the schema bounds the position
		in.Position = &p
	}
	return in
}

func (s *server) CreateLoginRule(w http.ResponseWriter, r *http.Request) {
	if s.o.Authn == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "login rules are not configured on this instance", nil)
		return
	}
	if !s.mayManageRights(w, r) {
		return
	}
	var body gen.LoginRuleInput
	if !decode(w, r, &body) {
		return
	}
	in := ruleInput(body)
	if !s.loginRuleNode(w, r, &in) {
		return
	}
	rule, err := s.o.Authn.CreateRule(r.Context(), in, actor(r))
	s.changed(r, "login_rule.create", rule.ID.String(), nil, rule, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/auth/rules/"+rule.ID.String())
	writeJSON(w, http.StatusCreated, rule)
}

func (s *server) UpdateLoginRule(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if s.o.Authn == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "login rules are not configured on this instance", nil)
		return
	}
	if !s.mayManageRights(w, r) {
		return
	}
	var body gen.LoginRuleInput
	if !decode(w, r, &body) {
		return
	}
	in := ruleInput(body)
	if !s.loginRuleInTenant(w, r, id) || !s.loginRuleNode(w, r, &in) {
		return
	}
	before, _ := s.o.Authn.Rule(r.Context(), id)
	rule, err := s.o.Authn.UpdateRule(r.Context(), id, in, actor(r))
	s.changed(r, "login_rule.update", id.String(), before, rule, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *server) DeleteLoginRule(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if s.o.Authn == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "login rules are not configured on this instance", nil)
		return
	}
	if !s.mayManageRights(w, r) {
		return
	}
	if !s.loginRuleInTenant(w, r, id) {
		return
	}
	before, _ := s.o.Authn.Rule(r.Context(), id)
	err := s.o.Authn.DeleteRule(r.Context(), id)
	s.changed(r, "login_rule.delete", id.String(), before, nil, err)
	s.noContent(w, r, err)
}
