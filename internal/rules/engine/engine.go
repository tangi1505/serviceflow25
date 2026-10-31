// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package engine runs the rules "if, then" (WF4; Hauptplan KF7, E111; architektur.md B4,
// §6.2). A rule belongs to a type and fires on create, update (transitions included) or a
// schedule; its condition is CEL over `record` and `changed`. A rule "before" changes the
// record in the same write (set, assign); a rule "after" runs as a job once the write is done
// (set, assign, create, notify), each match a write of its own with the rule in the history.
// Rules act with the rights of the system. Chains of rules stop at MaxDepth; a rule does not
// fire on its own write.
package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/rules"
)

// Triggers and modes of a rule.
const (
	TriggerCreate = records.TriggerCreate
	TriggerUpdate = records.TriggerUpdate
	TriggerTime   = "time"
	// TriggerClock is an escalation level (WF10): the rule fires once when a clock of its SLA
	// reaches Percent of its target.
	TriggerClock = "clock"
	ModeBefore   = "before"
	ModeAfter    = "after"
)

// MaxDepth ends a chain of rules: a write of a rule at this depth fires no further rules.
const MaxDepth = 5

// MaxMatches bounds the records one run of a rule on a schedule works on.
const MaxMatches = 1000

// channel tells the other instances that the rules changed.
const channel = "rules.changed"

// reloadAfter bounds how long an instance keeps rules without hearing of a change.
const reloadAfter = 30 * time.Second

// Rule is a rule as the API shows it.
type Rule struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Label       meta.Label `json:"label"`
	Description meta.Label `json:"description"`
	TypeName    string     `json:"type_name"`
	Trigger     string     `json:"trigger"`
	Mode        string     `json:"mode"`
	Condition   string     `json:"condition"`
	Schedule    string     `json:"schedule"`
	// Tenant is the tenant the rule acts in; empty for a rule of the installation, which acts
	// in every tenant (PL45).
	Tenant string `json:"tenant,omitempty"`
	// SLA and Percent or Minutes make the rule an escalation level (trigger clock, WF10):
	// it fires at a share of the target or after working minutes (WF11).
	SLA       *uuid.UUID `json:"sla"`
	Percent   *int       `json:"percent"`
	Minutes   *int       `json:"minutes"`
	Actions   []Action   `json:"actions"`
	Position  int        `json:"position"`
	Stop      bool       `json:"stop"`
	Enabled   bool       `json:"enabled"`
	LastRun   *time.Time `json:"last_run,omitempty"`
	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Action is one thing a rule does; exactly one of the five is set.
type Action struct {
	// Set gives fields values (API shape).
	Set map[string]any `json:"set,omitempty"`
	// Assign puts holders into a role at the record (WF3).
	Assign *Assign `json:"assign,omitempty"`
	// Create makes a record of a type (after the write only).
	Create *Create `json:"create,omitempty"`
	// Notify leaves a notification for the record (after the write only; WF15 mails it).
	Notify *Notify `json:"notify,omitempty"`
	// Transition moves the record along its state model (after the write only; WF11).
	Transition *Move `json:"transition,omitempty"`
}

// Move is a transition a rule makes, with the fields it sets on the way (WF2).
type Move struct {
	Name   string         `json:"name"`
	Fields map[string]any `json:"fields,omitempty"`
}

// Assign names a role and its holders; Add keeps the holders the record has.
type Assign struct {
	Role string `json:"role"`
	To   []Ref  `json:"to"`
	Add  bool   `json:"add,omitempty"`
}

// Ref is a holder of a role or the recipient of a notification.
type Ref struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

// Create is a record a rule makes.
type Create struct {
	TypeName string         `json:"type_name"`
	Fields   map[string]any `json:"fields"`
}

// Notify says whom to tell what: "actors.<role>" for the holders of a role at the record, or
// "<package.type>:<id>" for a person or group.
type Notify struct {
	To      []string `json:"to"`
	Message string   `json:"message"`
}

// compiled is a rule with its condition ready.
type compiled struct {
	Rule
	cond *rules.Condition
}

// reaches reports whether the rule acts on a record at the node: a rule of the installation
// everywhere, a rule of a tenant only there (PL45).
func (r Rule) reaches(path string) bool {
	return r.Tenant == "" || path == r.Tenant || strings.HasPrefix(path, r.Tenant+".")
}

// only keeps the rules that act on a record at the node.
func only(list []*compiled, path string) []*compiled {
	out := list[:0:0]
	for _, c := range list {
		if c.reaches(path) {
			out = append(out, c)
		}
	}
	return out
}

// scope is the tenant of the request for the rules it sees and changes: none for the system
// and an installation without tenants (PL45).
func scope(ctx context.Context) pgtype.Text {
	if a := authz.From(ctx); a.Bounded() {
		return pgtype.Text{String: a.CurrentTenant(), Valid: true}
	}
	return pgtype.Text{}
}

// Engine keeps the rules and runs them.
type Engine struct {
	db     *db.Router
	rec    *records.Service
	jobs   *jobs.Service
	kv     *cache.Client
	logger *slog.Logger
	// Queue is the River queue of the rules after a write; tests use their own.
	Queue string

	mu     sync.Mutex
	loaded time.Time
	byType map[string][]*compiled
}

// New wires the engine; kv may be nil (then only this instance hears of changes).
func New(rt *db.Router, rec *records.Service, js *jobs.Service, kv *cache.Client, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{db: rt, rec: rec, jobs: js, kv: kv, logger: logger, Queue: jobs.QueueDefault}
}

// ruleDB is the query handle in the database of a rule (PL47b): the tenant's, or the control
// database for a rule of the installation.
func (e *Engine) ruleDB(tenant pgtype.Text) *queries.Queries {
	if !tenant.Valid {
		return queries.New(e.db.Control())
	}
	return queries.New(e.db.For(tenant.String))
}

// allRules lists the rules of every database (PL47b), by type and position; with one database
// that is one query, as before.
func (e *Engine) allRules(ctx context.Context) ([]queries.Rule, error) {
	var out []queries.Rule
	for _, pool := range e.db.Pools() {
		rows, err := queries.New(pool).ListRules(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	if e.db.Separate() {
		slices.SortStableFunc(out, func(a, b queries.Rule) int {
			if c := strings.Compare(a.TypeName, b.TypeName); c != 0 {
				return c
			}
			if c := cmp.Compare(a.Position, b.Position); c != 0 {
				return c
			}
			return strings.Compare(a.Name, b.Name)
		})
	}
	return out, nil
}

// getRule finds the rule of that name the request sees: the tenant's own before the
// installation's (PL45), in the tenant's database, then the control database (PL47b).
func (e *Engine) getRule(ctx context.Context, name string) (queries.Rule, error) {
	pools := []*pgxpool.Pool{e.db.Tenant(ctx)}
	if c := e.db.Control(); c != pools[0] {
		pools = append(pools, c)
	}
	err := pgx.ErrNoRows
	for _, pool := range pools {
		var row queries.Rule
		row, err = queries.New(pool).GetRule(ctx, queries.GetRuleParams{Name: name, Tenant: scope(ctx)})
		if err == nil || !errors.Is(err, pgx.ErrNoRows) {
			return row, err
		}
	}
	return queries.Rule{}, err
}

// Run keeps the rules in step with the other instances until the context ends.
func (e *Engine) Run(ctx context.Context) error {
	if e.kv == nil {
		<-ctx.Done()
		return nil
	}
	for ctx.Err() == nil {
		err := e.kv.Subscribe(ctx, channel, func(string) { e.forget() })
		if err != nil && ctx.Err() == nil {
			e.logger.Warn("rules: subscription lost, retrying", "error", err.Error())
			time.Sleep(time.Second)
		}
	}
	return nil
}

func (e *Engine) forget() {
	e.mu.Lock()
	e.byType = nil
	e.mu.Unlock()
}

// changed drops the rules here and on the other instances.
func (e *Engine) changed(ctx context.Context) {
	e.forget()
	if e.kv != nil {
		if err := e.kv.Publish(ctx, channel, "1"); err != nil {
			e.logger.Warn("rules: publish", "error", err.Error())
		}
	}
}

// of returns the enabled rules of a type for a trigger and mode, in order.
func (e *Engine) of(ctx context.Context, typeName, trigger, mode string) ([]*compiled, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.byType == nil || time.Since(e.loaded) > reloadAfter {
		rows, err := e.allRules(ctx)
		if err != nil {
			return nil, fmt.Errorf("rules: load: %w", err)
		}
		byType := map[string][]*compiled{}
		for _, row := range rows {
			if !row.Enabled {
				continue
			}
			r, err := fromRow(row)
			if err != nil {
				e.logger.Warn("rules: unreadable rule", "rule", row.Name, "error", err.Error())
				continue
			}
			c := &compiled{Rule: r}
			if r.Condition != "" {
				if c.cond, err = rules.CompileRule(r.Condition); err != nil {
					e.logger.Warn("rules: condition does not compile", "rule", r.Name, "error", err.Error())
					continue
				}
			}
			byType[r.TypeName] = append(byType[r.TypeName], c)
		}
		e.byType, e.loaded = byType, time.Now()
	}
	var out []*compiled
	for _, c := range e.byType[typeName] {
		if c.Trigger == trigger && c.Mode == mode {
			out = append(out, c)
		}
	}
	return out, nil
}

// matches evaluates a rule's condition; a condition that cannot decide (a missing field)
// does not match and is logged.
func (e *Engine) matches(c *compiled, rec map[string]any, changed []string) bool {
	ok, err := c.cond.EvalRule(rec, changed)
	if err != nil {
		e.logger.Warn("rules: condition failed", "rule", c.Name, "error", err.Error())
		return false
	}
	return ok
}

// Before is the hook of records (records.BeforeWrite): the rules "before" of the type and
// trigger, in order, each seeing what the ones before it set.
func (e *Engine) Before(ctx context.Context, t meta.Type, trigger string, rec map[string]any, changed []string) (records.RuleChange, error) {
	var rc records.RuleChange
	list, err := e.of(ctx, t.QualifiedName(), trigger, ModeBefore)
	if err != nil {
		return rc, err
	}
	path, _ := rec["org_path"].(string)
	if list = only(list, path); len(list) == 0 {
		return rc, nil
	}
	current := make(map[string]any, len(rec))
	for k, v := range rec {
		current[k] = v
	}
	for _, c := range list {
		if !e.matches(c, current, changed) {
			continue
		}
		set, roles := e.changes(c.Rule, current)
		for k, v := range set {
			if rc.Set == nil {
				rc.Set, rc.By = map[string]any{}, map[string]string{}
			}
			rc.Set[k], rc.By[k], current[k] = v, c.Name, v
		}
		for role, holders := range roles {
			if rc.Actors == nil {
				rc.Actors = map[string]any{}
			}
			if rc.By == nil {
				rc.By = map[string]string{}
			}
			rc.Actors[role], rc.By["actors."+role] = holders, c.Name
			setRole(current, role, holders)
		}
		if c.Stop {
			break
		}
	}
	return rc, nil
}

// changes turns the set and assign actions of a rule into field values and whole roles,
// against the record as it is.
func (e *Engine) changes(r Rule, rec map[string]any) (map[string]any, map[string]any) {
	set, roles := map[string]any{}, map[string]any{}
	for _, a := range r.Actions {
		for k, v := range a.Set {
			set[k] = v
		}
		if a.Assign == nil {
			continue
		}
		var holders []any
		if a.Assign.Add {
			if prev, ok := roles[a.Assign.Role].([]any); ok {
				holders = prev
			} else {
				holders = append(holders, holdersOf(rec, a.Assign.Role)...)
			}
		}
		for _, ref := range a.Assign.To {
			holders = append(holders, map[string]any{"type": ref.Type, "id": ref.ID.String()})
		}
		if holders == nil {
			holders = []any{}
		}
		roles[a.Assign.Role] = holders
	}
	return set, roles
}

func holdersOf(rec map[string]any, role string) []any {
	all, _ := rec["actors"].(map[string]any)
	list, _ := all[role].([]any)
	return append([]any{}, list...)
}

func setRole(rec map[string]any, role string, holders any) {
	all, _ := rec["actors"].(map[string]any)
	copied := make(map[string]any, len(all)+1)
	for k, v := range all {
		copied[k] = v
	}
	copied[role] = holders
	rec["actors"] = copied
}

// --- managing rules ---

// List returns the rules the request sees, by type and position: in a tenant its own and those
// of the installation (PL45).
func (e *Engine) List(ctx context.Context) ([]Rule, error) {
	rows, err := e.allRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("rules: list: %w", err)
	}
	sc := scope(ctx)
	out := make([]Rule, 0, len(rows))
	for _, row := range rows {
		if sc.Valid && row.Tenant.Valid && row.Tenant.String != sc.String {
			continue
		}
		r, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Get returns the rule of that name the request sees: the tenant's own, else the
// installation's (PL45).
func (e *Engine) Get(ctx context.Context, name string) (Rule, error) {
	row, err := e.getRule(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, fmt.Errorf("rule %s: %w", name, records.ErrNotFound)
	}
	if err != nil {
		return Rule{}, fmt.Errorf("rules: get: %w", err)
	}
	return fromRow(row)
}

// Put creates the rule of that name or replaces it; created tells which. A new rule belongs to
// the tenant of the request, or with installation to the installation (PL45); a name stays
// with the rule that has it.
func (e *Engine) Put(ctx context.Context, name string, r Rule, installation bool, actor *uuid.UUID) (Rule, bool, error) {
	r.Name = name
	target := scope(ctx)
	if installation {
		target = pgtype.Text{}
	}
	existing, err := e.getRule(ctx, name)
	switch {
	case err == nil && existing.Tenant != target:
		return Rule{}, false, fmt.Errorf("%w: the name %s belongs to a rule of %s", records.ErrConflict, name, owner(existing.Tenant))
	case errors.Is(err, pgx.ErrNoRows) && !target.Valid:
		if taken, err := e.nameInTenants(ctx, name); err != nil {
			return Rule{}, false, err
		} else if taken {
			return Rule{}, false, fmt.Errorf("%w: a tenant has a rule named %s", records.ErrConflict, name)
		}
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return Rule{}, false, fmt.Errorf("rules: get: %w", err)
	}
	if r.Label == nil {
		r.Label = meta.Label{}
	}
	if r.Description == nil {
		r.Description = meta.Label{}
	}
	if err := e.check(ctx, &r); err != nil {
		return Rule{}, false, err
	}
	label, _ := json.Marshal(r.Label)
	desc, _ := json.Marshal(r.Description)
	actions, _ := json.Marshal(r.Actions)
	var by uuid.NullUUID
	if actor != nil {
		by = uuid.NullUUID{UUID: *actor, Valid: true}
	}
	sla, percent, minutes := levelParams(r)
	var row queries.Rule
	created := existing.ID == uuid.Nil
	if created {
		id, idErr := uuid.NewV7()
		if idErr != nil {
			return Rule{}, false, idErr
		}
		row, err = e.ruleDB(target).InsertRule(ctx, queries.InsertRuleParams{ID: id, Name: name, Label: label, Description: desc, TypeName: r.TypeName,
			Trigger: r.Trigger, Mode: r.Mode, Condition: r.Condition, Schedule: r.Schedule, Actions: actions,
			Position: int32(r.Position), Stop: r.Stop, Enabled: r.Enabled, Sla: sla, Percent: percent, Minutes: minutes, Actor: by, Tenant: target}) //nolint:gosec // checked to fit
	} else {
		row, err = e.ruleDB(existing.Tenant).UpdateRule(ctx, queries.UpdateRuleParams{ID: existing.ID, Label: label, Description: desc, TypeName: r.TypeName,
			Trigger: r.Trigger, Mode: r.Mode, Condition: r.Condition, Schedule: r.Schedule, Actions: actions,
			Position: int32(r.Position), Stop: r.Stop, Enabled: r.Enabled, Sla: sla, Percent: percent, Minutes: minutes, Actor: by}) //nolint:gosec // checked to fit
	}
	if err != nil {
		return Rule{}, false, fmt.Errorf("rules: save: %w", err)
	}
	e.changed(ctx)
	out, err := fromRow(row)
	return out, created, err
}

// nameInTenants reports whether a tenant has a rule of that name, in any database (PL47b).
func (e *Engine) nameInTenants(ctx context.Context, name string) (bool, error) {
	for _, pool := range e.db.Pools() {
		taken, err := queries.New(pool).RuleNameInTenants(ctx, name)
		if err != nil || taken {
			return taken, err
		}
	}
	return false, nil
}

// owner names who a rule belongs to, for messages.
func owner(tenant pgtype.Text) string {
	if tenant.Valid {
		return "the tenant " + tenant.String
	}
	return "the installation"
}

// Delete removes the rule of that name the request sees.
func (e *Engine) Delete(ctx context.Context, name string) error {
	r, err := e.Get(ctx, name)
	if err != nil {
		return err
	}
	n, err := e.ruleDB(pgtype.Text{String: r.Tenant, Valid: r.Tenant != ""}).DeleteRule(ctx, r.ID)
	if err != nil {
		return fmt.Errorf("rules: delete: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("rule %s: %w", name, records.ErrNotFound)
	}
	e.changed(ctx)
	return nil
}

// check validates a rule before it is saved: every problem at once, as a validation error.
func (e *Engine) check(ctx context.Context, r *Rule) error {
	var ps []records.Problem
	add := func(field, code, format string, args ...any) {
		ps = append(ps, records.Problem{Field: field, Code: code, Message: fmt.Sprintf(format, args...)})
	}
	if !meta.IsIdent(r.Name) {
		add("name", "type", "must be snake_case (a-z, 0-9, _), at most 63 characters")
	}
	t, err := e.rec.Type(ctx, r.TypeName)
	if err != nil {
		add("type_name", "unknown", "%q is not an installed type", r.TypeName)
	}
	if r.Trigger != TriggerClock && (r.SLA != nil || r.Percent != nil || r.Minutes != nil) {
		add("sla", "type", "only escalation levels (trigger clock) watch an SLA")
	}
	switch r.Trigger {
	case TriggerCreate, TriggerUpdate:
		if r.Schedule != "" {
			add("schedule", "type", "only rules on a schedule (trigger time) have one")
		}
	case TriggerClock:
		if r.Mode != ModeAfter {
			add("mode", "type", "an escalation level runs after, as a job")
		}
		if r.Schedule != "" {
			add("schedule", "type", "only rules on a schedule (trigger time) have one")
		}
		switch {
		case (r.Percent == nil) == (r.Minutes == nil):
			add("percent", "type", "an escalation level needs either a share of the target (percent) or working minutes (minutes)")
		case r.Percent != nil && (*r.Percent < 1 || *r.Percent > 1000):
			add("percent", "type", "an escalation level needs a share of the target from 1 to 1000 percent")
		case r.Minutes != nil && (*r.Minutes < 1 || *r.Minutes > 5_256_000):
			add("minutes", "type", "an escalation level needs from 1 minute to ten years of working time")
		}
		if r.SLA == nil {
			add("sla", "required", "an escalation level needs the SLA it watches")
		} else if sla, err := e.rec.Get(authz.WithAccess(ctx, authz.System()), "core.sla", *r.SLA); err != nil {
			add("sla", "unknown", "%s is not an SLA", *r.SLA)
		} else if sla["target_type"] != r.TypeName {
			add("sla", "type", "the SLA %s is for %v, not for %s", sla["name"], sla["target_type"], r.TypeName)
		}
	case TriggerTime:
		if r.Mode != ModeAfter {
			add("mode", "type", "a rule on a schedule runs after, as a job")
		}
		if _, err := cron.ParseStandard(r.Schedule); err != nil || strings.TrimSpace(r.Schedule) == "" {
			add("schedule", "type", "must be a cron expression like \"0 * * * *\" or @hourly")
		}
		if r.Condition != "" {
			if _, _, err := authz.Translate(r.Condition, nil); err != nil {
				add("condition", "type", "a rule on a schedule finds its records in SQL; this condition cannot be translated: %v", err)
			}
		}
	default:
		add("trigger", "type", "must be create, update, time or clock")
	}
	if r.Mode != ModeBefore && r.Mode != ModeAfter {
		add("mode", "type", "must be before or after")
	}
	if r.Condition != "" {
		if _, err := rules.CompileRule(r.Condition); err != nil {
			add("condition", "type", "%v", err)
		}
	}
	if r.Position < -1_000_000 || r.Position > 1_000_000 {
		add("position", "type", "must lie between -1000000 and 1000000")
	}
	if len(r.Actions) == 0 {
		add("actions", "required", "a rule needs at least one action")
	}
	for i, a := range r.Actions {
		where := fmt.Sprintf("actions[%d]", i)
		n := 0
		if a.Set != nil {
			n++
		}
		if a.Assign != nil {
			n++
		}
		if a.Create != nil {
			n++
		}
		if a.Notify != nil {
			n++
		}
		if a.Transition != nil {
			n++
		}
		if n != 1 {
			add(where, "type", "an action is exactly one of set, assign, create, notify or transition")
			continue
		}
		if r.Mode == ModeBefore && (a.Create != nil || a.Notify != nil || a.Transition != nil) {
			add(where, "type", "before the write a rule can only set and assign; create, notify and transition run after")
		}
		if err != nil {
			continue
		}
		switch {
		case a.Set != nil:
			for name := range a.Set {
				if f, ok := t.Field(name); ok && f.Calculated() {
					add(where, "read_only", "%q is calculated by the core", name)
				}
			}
			if err := e.rec.CheckValues(ctx, r.TypeName, a.Set, false); err != nil {
				add(where, "invalid", "%v", err)
			}
		case a.Assign != nil:
			holders := make([]any, len(a.Assign.To))
			for j, ref := range a.Assign.To {
				holders[j] = map[string]any{"type": ref.Type, "id": ref.ID.String()}
			}
			if err := e.rec.CheckValues(ctx, r.TypeName, map[string]any{"actors": map[string]any{a.Assign.Role: holders}}, false); err != nil {
				add(where, "invalid", "%v", err)
			}
		case a.Create != nil:
			if err := e.rec.CheckValues(ctx, a.Create.TypeName, a.Create.Fields, true); err != nil {
				add(where, "invalid", "%v", err)
			}
		case a.Transition != nil:
			if _, ok := t.Transition(a.Transition.Name); !ok {
				add(where, "unknown", "%q is not a transition of %s", a.Transition.Name, r.TypeName)
			} else if len(a.Transition.Fields) > 0 {
				if err := e.rec.CheckValues(ctx, r.TypeName, a.Transition.Fields, false); err != nil {
					add(where, "invalid", "%v", err)
				}
			}
		case a.Notify != nil:
			if len(a.Notify.To) == 0 || strings.TrimSpace(a.Notify.Message) == "" {
				add(where, "required", "a notification needs recipients and a message")
			}
			for _, to := range a.Notify.To {
				if role, ok := strings.CutPrefix(to, "actors."); ok {
					if _, known := t.Actor(role); !known {
						add(where, "unknown", "%q is not a role of %s", role, r.TypeName)
					}
					continue
				}
				typ, id, ok := strings.Cut(to, ":")
				if _, err := uuid.Parse(id); !ok || err != nil || !strings.Contains(typ, ".") {
					add(where, "type", "recipient %q must be actors.<role> or <package.type>:<id>", to)
				}
			}
		}
	}
	if len(ps) > 0 {
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].Field < ps[j].Field })
		return &records.ValidationError{Problems: ps}
	}
	return nil
}

func fromRow(row queries.Rule) (Rule, error) {
	r := Rule{ID: row.ID, Name: row.Name, TypeName: row.TypeName, Trigger: row.Trigger, Mode: row.Mode, Condition: row.Condition,
		Schedule: row.Schedule, Position: int(row.Position), Stop: row.Stop, Enabled: row.Enabled, Version: row.Version,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, Tenant: row.Tenant.String}
	if row.LastRun.Valid {
		t := row.LastRun.Time
		r.LastRun = &t
	}
	if row.Sla.Valid {
		id := row.Sla.UUID
		r.SLA = &id
	}
	if row.Percent.Valid {
		n := int(row.Percent.Int32)
		r.Percent = &n
	}
	if row.Minutes.Valid {
		n := int(row.Minutes.Int32)
		r.Minutes = &n
	}
	if err := json.Unmarshal(row.Label, &r.Label); err != nil {
		return Rule{}, fmt.Errorf("rules: label of %s: %w", row.Name, err)
	}
	if err := json.Unmarshal(row.Description, &r.Description); err != nil {
		return Rule{}, fmt.Errorf("rules: description of %s: %w", row.Name, err)
	}
	if err := json.Unmarshal(row.Actions, &r.Actions); err != nil {
		return Rule{}, fmt.Errorf("rules: actions of %s: %w", row.Name, err)
	}
	return r, nil
}

// levelParams are the SLA, percent and minutes of an escalation level as the table keeps them.
func levelParams(r Rule) (uuid.NullUUID, pgtype.Int4, pgtype.Int4) {
	var sla uuid.NullUUID
	var percent, minutes pgtype.Int4
	if r.SLA != nil {
		sla = uuid.NullUUID{UUID: *r.SLA, Valid: true}
	}
	if r.Percent != nil {
		percent = pgtype.Int4{Int32: int32(*r.Percent), Valid: true} //nolint:gosec // checked: 1 to 1000
	}
	if r.Minutes != nil {
		minutes = pgtype.Int4{Int32: int32(*r.Minutes), Valid: true} //nolint:gosec // checked: at most ten years
	}
	return sla, percent, minutes
}

// Level is an escalation level as the clocks see it: a share of the target or working
// minutes.
type Level struct {
	Rule    string
	Percent int
	Minutes int
}

// Levels returns the enabled escalation levels of an SLA for a type (WF10, WF11); the clocks
// plan their jobs by them (sla.Service.Levels).
func (e *Engine) Levels(ctx context.Context, typeName string, sla uuid.UUID) ([]Level, error) {
	list, err := e.of(ctx, typeName, TriggerClock, ModeAfter)
	if err != nil {
		return nil, err
	}
	var out []Level
	for _, c := range list {
		if c.SLA == nil || *c.SLA != sla {
			continue
		}
		l := Level{Rule: c.Name}
		if c.Percent != nil {
			l.Percent = *c.Percent
		}
		if c.Minutes != nil {
			l.Minutes = *c.Minutes
		}
		out = append(out, l)
	}
	return out, nil
}
