// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/rules"
)

// Rule assigns a role at a node when its condition over the claims holds (G121, E88).
type Rule struct {
	ID        uuid.UUID  `json:"id"`
	Position  int32      `json:"position"`
	Name      string     `json:"name"`
	Condition string     `json:"condition"`
	Role      string     `json:"role"`
	OrgPath   string     `json:"org_path"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	UpdatedBy *uuid.UUID `json:"updated_by,omitempty"`
}

// RuleInput is a rule as the API takes it.
type RuleInput struct {
	Name      string
	Condition string
	Role      string
	OrgPath   string
	Enabled   *bool
	Position  *int32
}

func ruleOf(r queries.ListLoginRulesRow) Rule {
	out := Rule{ID: r.ID, Position: r.Position, Name: r.Name, Condition: r.Condition, Role: r.RoleName, OrgPath: r.OrgPath,
		Enabled: r.Enabled, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.UpdatedBy.Valid {
		by := r.UpdatedBy.UUID
		out.UpdatedBy = &by
	}
	return out
}

// Rules lists the login rules in their order.
func (s *Service) Rules(ctx context.Context) ([]Rule, error) {
	rows, err := queries.New(s.db.Control()).ListLoginRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("authn: rules: %w", err)
	}
	out := make([]Rule, 0, len(rows))
	for _, r := range rows {
		out = append(out, ruleOf(r))
	}
	return out, nil
}

// Rule returns one rule.
func (s *Service) Rule(ctx context.Context, id uuid.UUID) (Rule, error) {
	r, err := queries.New(s.db.Control()).GetLoginRule(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, fmt.Errorf("%w: rule %s", ErrNotFound, id)
	}
	if err != nil {
		return Rule{}, fmt.Errorf("authn: rule: %w", err)
	}
	return ruleOf(queries.ListLoginRulesRow(r)), nil
}

// check validates a rule: the condition compiles over the claims, the role and the node exist.
func (s *Service) check(ctx context.Context, in RuleInput) (roleID uuid.UUID, orgPath string, err error) {
	if in.Name == "" || in.Condition == "" || in.Role == "" {
		return uuid.Nil, "", fmt.Errorf("%w: a rule needs name, condition and role", ErrInvalid)
	}
	if _, err := rules.CompileClaims(in.Condition); err != nil {
		return uuid.Nil, "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	role, err := queries.New(s.db.Control()).GetRoleByName(ctx, in.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", fmt.Errorf("%w: no role %q", ErrInvalid, in.Role)
	}
	if err != nil {
		return uuid.Nil, "", err
	}
	orgPath = in.OrgPath
	if orgPath == "" {
		orgPath = "root"
	}
	if _, err := s.org.Node(ctx, orgPath); err != nil {
		return uuid.Nil, "", fmt.Errorf("%w: no node %q", ErrInvalid, orgPath)
	}
	return role.ID, orgPath, nil
}

// CreateRule stores a rule.
func (s *Service) CreateRule(ctx context.Context, in RuleInput, by *uuid.UUID) (Rule, error) {
	roleID, orgPath, err := s.check(ctx, in)
	if err != nil {
		return Rule{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Rule{}, err
	}
	enabled, position := true, int32(100)
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if in.Position != nil {
		position = *in.Position
	}
	err = queries.New(s.db.Control()).InsertLoginRule(ctx, queries.InsertLoginRuleParams{
		ID: id, Position: position, Name: in.Name, Condition: in.Condition, RoleID: roleID, OrgPath: orgPath,
		Enabled: enabled, UpdatedBy: nullUUID(by),
	})
	if err != nil {
		return Rule{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return s.Rule(ctx, id)
}

// UpdateRule replaces a rule.
func (s *Service) UpdateRule(ctx context.Context, id uuid.UUID, in RuleInput, by *uuid.UUID) (Rule, error) {
	current, err := s.Rule(ctx, id)
	if err != nil {
		return Rule{}, err
	}
	roleID, orgPath, err := s.check(ctx, in)
	if err != nil {
		return Rule{}, err
	}
	enabled, position := current.Enabled, current.Position
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if in.Position != nil {
		position = *in.Position
	}
	n, err := queries.New(s.db.Control()).UpdateLoginRule(ctx, queries.UpdateLoginRuleParams{
		ID: id, Position: position, Name: in.Name, Condition: in.Condition, RoleID: roleID, OrgPath: orgPath,
		Enabled: enabled, UpdatedBy: nullUUID(by),
	})
	if err != nil {
		return Rule{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if n == 0 {
		return Rule{}, fmt.Errorf("%w: rule %s", ErrNotFound, id)
	}
	return s.Rule(ctx, id)
}

// DeleteRule removes a rule and the assignments it made, so the rights cache is invalidated.
// The rule is central, its assignments are in the database of the tenant of its node (PL47b).
func (s *Service) DeleteRule(ctx context.Context, id uuid.UUID) error {
	rule, err := s.Rule(ctx, id)
	if err != nil {
		return err
	}
	if _, err := queries.New(s.db.For(rule.OrgPath)).DeleteAssignmentsOfLoginRule(ctx, uuid.NullUUID{UUID: id, Valid: true}); err != nil {
		return fmt.Errorf("authn: assignments of the rule: %w", err)
	}
	n, err := queries.New(s.db.Control()).DeleteLoginRule(ctx, id)
	if err != nil {
		return fmt.Errorf("authn: delete rule: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: rule %s", ErrNotFound, id)
	}
	s.az.Invalidate(ctx)
	return nil
}

func nullUUID(u *uuid.UUID) uuid.NullUUID {
	if u == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *u, Valid: true}
}

// matching lists the enabled login rules the claims match, in their order.
func (s *Service) matching(ctx context.Context, claims Claims) ([]queries.ListLoginRulesRow, error) {
	list, err := queries.New(s.db.Control()).ListLoginRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("authn: rules: %w", err)
	}
	var out []queries.ListLoginRulesRow
	for _, r := range list {
		if !r.Enabled {
			continue
		}
		cond, err := rules.CompileClaims(r.Condition)
		if err != nil {
			s.logger.Warn("login rule does not compile, skipped", "rule", r.Name, "error", err.Error())
			continue
		}
		ok, err := cond.Eval(map[string]any(claims))
		if err != nil {
			// A claim the provider did not send: the rule does not match.
			s.logger.Debug("login rule not evaluable", "rule", r.Name, "error", err.Error())
			continue
		}
		if ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// tenantByRules is the tenant of the first login rule the claims match, "" without tenants or
// without a match (PL45).
func (s *Service) tenantByRules(ctx context.Context, claims Claims) (string, error) {
	tenants, err := org.TenantPaths(ctx, queries.New(s.db.Control()))
	if err != nil || len(tenants) == 0 {
		return "", err
	}
	list, err := s.matching(ctx, claims)
	if err != nil {
		return "", err
	}
	for _, r := range list {
		if t := org.TenantOf(tenants, r.OrgPath); t != org.Root {
			return t, nil
		}
	}
	return "", nil
}

// ApplyRules brings the rule-made assignments of a person in line with the claims: a rule
// that matches makes sure its assignment exists, one that does not (or is off) takes its
// assignment away. Assignments made by hand are not touched. For an account of a tenant only
// the rules of that tenant count (PL45).
func (s *Service) ApplyRules(ctx context.Context, person uuid.UUID, tenant string, claims Claims) error {
	// The person's assignments are in the database of the account's tenant (PL47b).
	q := queries.New(s.db.For(tenant))
	list, err := s.matching(ctx, claims)
	if err != nil {
		return err
	}
	type target struct {
		roleID  uuid.UUID
		orgPath string
	}
	want := map[uuid.UUID]target{}
	for _, r := range list {
		if tenant == "" || r.OrgPath == tenant || strings.HasPrefix(r.OrgPath, tenant+".") {
			want[r.ID] = target{r.RoleID, r.OrgPath}
		}
	}
	existing, err := q.ListRuleAssignmentsOfPerson(ctx, uuid.NullUUID{UUID: person, Valid: true})
	if err != nil {
		return fmt.Errorf("authn: rule assignments: %w", err)
	}
	changed := false
	have := map[uuid.UUID]bool{}
	for _, a := range existing {
		w, ok := want[a.RuleID.UUID]
		if ok && w.roleID == a.RoleID && w.orgPath == a.OrgPath {
			have[a.RuleID.UUID] = true
			continue
		}
		if _, err := q.DeleteAssignment(ctx, a.ID); err != nil {
			return fmt.Errorf("authn: remove assignment: %w", err)
		}
		changed = true
	}
	for ruleID, w := range want {
		if have[ruleID] {
			continue
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if err := q.InsertRuleAssignment(ctx, queries.InsertRuleAssignmentParams{
			ID: id, RoleID: w.roleID, PersonID: uuid.NullUUID{UUID: person, Valid: true}, OrgPath: w.orgPath,
			RuleID: uuid.NullUUID{UUID: ruleID, Valid: true},
		}); err != nil {
			return fmt.Errorf("authn: assign by rule: %w", err)
		}
		changed = true
	}
	if changed {
		s.az.Invalidate(ctx)
	}
	return nil
}
