// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/branding"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/rules/engine"
	"group24.de/itsm/internal/seed"
)

// --- upgrade steps of packages (WF7, WF-Plan E18) ---
//
// When a package update gives up one of its own types, the data of that type moves once, per
// tenant, after the new definitions, tables and start data are in place. A step runs when the
// installed version is below the one named; it must be safe to run again.

type upgrade struct {
	pkg   string
	below string
	name  string
	run   func(ctx context.Context, u upgradeEnv, scope string) (int, error)
}

// upgradeEnv is what a step works with.
type upgradeEnv struct {
	rt    *db.Router
	recs  *records.Service
	seeds *seed.Service
	rules *engine.Engine
	p     meta.Package
}

var upgrades = []upgrade{
	{pkg: "request", below: "0.3.0", name: "Genehmigungsstufen der Erfüllungsvorlagen in Genehmigungsvorlagen", run: moveTemplateStages},
	{pkg: "incident", below: "0.9.0", name: "Automatisches Schließen aus der Einstellung incident.close_after", run: closeFromSetting},
	{pkg: "core", below: "0.10.0", name: "Benachrichtigungen als Mail und in den Posteingang", run: notificationWays},
	{pkg: "core", below: "0.13.0", name: "Akzentfarbe der Organisation als Farbe statt als Name", run: accentAsColor},
	{pkg: "sd", below: "0.5.3", name: "Statistik öffnet mit 7 Tagen", run: statistikPeriod},
}

// runUpgrades runs the steps of the package for an installed version below theirs.
func runUpgrades(ctx context.Context, u upgradeEnv, installed string, stdout io.Writer) error {
	if installed == "" {
		return nil
	}
	for _, step := range upgrades {
		if step.pkg != u.p.Name || !versionBelow(installed, step.below) {
			continue
		}
		scopes, err := u.seeds.Scopes(ctx)
		if err != nil {
			return err
		}
		total := 0
		for _, scope := range scopes {
			n, err := step.run(ctx, u, scope)
			total += n
			if err != nil {
				return fmt.Errorf("upgrade %s at %s: %w", step.name, scope, err)
			}
		}
		fmt.Fprintf(stdout, "Umzug „%s“ (von %s): %d umgezogen\n", step.name, installed, total)
	}
	return nil
}

// versionBelow compares versions like 0.2.0 number by number.
func versionBelow(v, than string) bool {
	a, b := strings.Split(v, "."), strings.Split(than, ".")
	for i := range max(len(a), len(b)) {
		var x, y int
		if i < len(a) {
			x, _ = strconv.Atoi(a[i])
		}
		if i < len(b) {
			y, _ = strconv.Atoi(b[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// notificationWays gives every notification from before WF19 both ways, mail and inbox (your
// decision 2026-11-16, WF-Plan E19): the new columns came empty.
func notificationWays(ctx context.Context, u upgradeEnv, scope string) (int, error) {
	ctx = u.rt.BindFor(authz.WithAccess(ctx, authz.SystemIn(scope)), scope)
	where, _ := json.Marshal(map[string]any{"or": []any{
		map[string]any{"field": "mail", "null": true},
		map[string]any{"field": "inbox", "null": true},
	}})
	moved := 0
	for {
		page, _, err := u.recs.Query(ctx, "core.notification", records.Query{Where: where, Limit: 200})
		if err != nil {
			return moved, err
		}
		if len(page.Items) == 0 {
			return moved, nil
		}
		for _, n := range page.Items {
			id, err := uuid.Parse(fmt.Sprint(n["id"]))
			if err != nil {
				return moved, err
			}
			version, _ := n["version"].(int64)
			if _, err := u.recs.Update(ctx, "core.notification", id, version, map[string]any{"mail": true, "inbox": true}, nil); err != nil {
				return moved, err
			}
			moved++
		}
	}
}

// accentAsColor turns the names of the six accents in ui.accent into their colour (UI39, UI-Plan
// E51): the setting is a colour #rrggbb now, so that the organisation may choose a free one.
func accentAsColor(ctx context.Context, u upgradeEnv, scope string) (int, error) {
	moved := 0
	for name, color := range branding.Named() {
		tag, err := u.rt.For(scope).Exec(ctx, `UPDATE org_setting SET value = to_jsonb($1::text), updated_at = now()
			WHERE key = 'ui.accent' AND path <@ $2::ltree AND value = to_jsonb($3::text)`, color, scope, name)
		if err != nil {
			return moved, err
		}
		moved += int(tag.RowsAffected())
	}
	return moved, nil
}

// moveTemplateStages makes a core.approval_template of the stages of every fulfilment template
// at the node that has stages and no approval template yet, names it at the fulfilment
// template, and puts the two rules of the start data that asked from the stages again as
// request 0.3 defines them, unless the tenant changed them.
func moveTemplateStages(ctx context.Context, u upgradeEnv, scope string) (int, error) {
	ctx = u.rt.BindFor(authz.WithAccess(ctx, authz.SystemIn(scope)), scope)
	// The stages of request 0.2 (request.template_stage): the type is gone, its table stays.
	stages, err := u.recs.RemovedRows(ctx, "request", "template_stage", scope)
	if err != nil {
		return 0, err
	}
	byTemplate := map[string][]records.Record{}
	var order []string
	for _, st := range stages {
		tid, _ := st["template"].(string)
		if tid == "" {
			continue
		}
		if _, ok := byTemplate[tid]; !ok {
			order = append(order, tid)
		}
		byTemplate[tid] = append(byTemplate[tid], st)
	}
	moved := 0
	for _, raw := range order {
		tid, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		tpl, err := u.recs.Get(ctx, "request.template", tid)
		if errors.Is(err, records.ErrNotFound) {
			continue
		}
		if err != nil {
			return moved, err
		}
		if tpl["approval_template"] != nil && tpl["approval_template"] != "" {
			continue
		}
		name, err := freeTemplateName(ctx, u.recs, fmt.Sprint(tpl["name"]))
		if err != nil {
			return moved, err
		}
		at, err := u.recs.Create(ctx, records.TypeApprovalTemplate, map[string]any{
			"name": name, "message": "Bitte genehmigen Sie diese Anfrage.", "active": true, "org_path": tpl["org_path"],
		}, nil)
		if err != nil {
			return moved, err
		}
		list := byTemplate[raw]
		slices.SortStableFunc(list, func(a, b records.Record) int {
			pa, _ := a["position"].(float64)
			pb, _ := b["position"].(float64)
			return cmp.Compare(pa, pb)
		})
		for _, st := range list {
			values := map[string]any{"template": at["id"], "name": st["name"], "actors": st["actors"], "org_path": tpl["org_path"]}
			if n, ok := st["need"].(float64); ok {
				values["need"] = n
			}
			if n, ok := st["position"].(float64); ok {
				values["position"] = n
			}
			if _, err := u.recs.Create(ctx, records.TypeApprovalTemplateStage, values, nil); err != nil {
				return moved, err
			}
			moved++
		}
		version, _ := tpl["version"].(int64)
		if _, err := u.recs.Update(ctx, "request.template", tid, version, map[string]any{"approval_template": at["id"]}, nil); err != nil {
			return moved, err
		}
	}
	// The rules of the start data, as long as they still ask from the old stages.
	var keys []string
	for _, key := range []string{"genehmigung_beim_anlegen", "genehmigung_bei_vorlage"} {
		r, err := u.rules.Get(ctx, seed.RuleName(u.p.Name, key, scope))
		if errors.Is(err, records.ErrNotFound) {
			continue
		}
		if err != nil {
			return moved, err
		}
		raw, _ := json.Marshal(r.Actions)
		if strings.Contains(string(raw), "request.template_stage") {
			keys = append(keys, key)
		}
	}
	if len(keys) > 0 {
		if _, err := u.seeds.Refresh(ctx, u.p, scope, keys...); err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// freeTemplateName is the name, or with a number when a template of that name exists.
func freeTemplateName(ctx context.Context, recs *records.Service, name string) (string, error) {
	try := name
	for i := 2; i < 100; i++ {
		page, err := recs.List(ctx, records.TypeApprovalTemplate, records.ListOptions{Limit: 1, Filter: map[string]string{"name": try}})
		if err != nil {
			return "", err
		}
		if len(page.Items) == 0 {
			return try, nil
		}
		try = fmt.Sprintf("%s (%d)", name, i)
	}
	return try, nil
}

// closeFromSetting lets the SLA of the start data that closes a resolved incident take its
// target from the setting incident.close_after (SD20), as incident 0.9 defines it, unless the
// tenant gave it a setting of its own already. The target in minutes stays as it is: it holds
// where no unit sets the time, and the default of the setting is the same 3300 minutes.
// statistikPeriod gives the dashboard Statistik of the start data the period of 7 days it opens
// with (PL65, Hauptplan E195), unless the tenant set one; other dashboards keep "Gesamt".
func statistikPeriod(ctx context.Context, u upgradeEnv, scope string) (int, error) {
	ref, err := u.seeds.Ref(ctx, scope, "sd", "statistik")
	if err != nil || ref == "" {
		return 0, err
	}
	id, err := uuid.Parse(ref)
	if err != nil {
		return 0, fmt.Errorf("sd.statistik at %s is no record: %w", scope, err)
	}
	ctx = u.rt.BindFor(authz.WithAccess(ctx, authz.SystemIn(scope)), scope)
	rec, err := u.recs.Get(ctx, "core.dashboard", id)
	if errors.Is(err, records.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if v, _ := rec["period"].(string); v != "" {
		return 0, nil
	}
	version, _ := rec["version"].(int64)
	if _, err := u.recs.Update(ctx, "core.dashboard", id, version, map[string]any{"period": "days_7"}, nil); err != nil {
		return 0, err
	}
	return 1, nil
}

func closeFromSetting(ctx context.Context, u upgradeEnv, scope string) (int, error) {
	ref, err := u.seeds.Ref(ctx, scope, "incident", "sla_close")
	if err != nil || ref == "" {
		return 0, err
	}
	id, err := uuid.Parse(ref)
	if err != nil {
		return 0, fmt.Errorf("incident.sla_close at %s is no record: %w", scope, err)
	}
	ctx = u.rt.BindFor(authz.WithAccess(ctx, authz.SystemIn(scope)), scope)
	rec, err := u.recs.Get(ctx, "core.sla", id)
	if errors.Is(err, records.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if v, _ := rec["target_setting"].(string); v != "" {
		return 0, nil
	}
	version, _ := rec["version"].(int64)
	if _, err := u.recs.Update(ctx, "core.sla", id, version, map[string]any{"target_setting": "incident.close_after"}, nil); err != nil {
		return 0, err
	}
	return 1, nil
}
