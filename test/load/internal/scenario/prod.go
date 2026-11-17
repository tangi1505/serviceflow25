// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package scenario

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// --- LZ1, LZ2 and LZ4 on the data set in Prod size (H14, Hauptplan §2.5) ---
//
// With --data=prod the full stage measures what §2.5 names: LZ1 reads random incidents of the
// 18 million, LZ2 is the queue "incidents of my team" for a person whose role reads only the
// incidents a group of hers takes part in, LZ4 creates incidents with the rules and clocks of
// the packages. test/load fill made the data: incidents of the source loadgen, each with a
// requester and one of the teams "Lasttest Team 00" to 49.

const incidentType = "incident.incident"

// ProdTeam is the team LZ2 works in.
const ProdTeam = "Lasttest Team 07"

// prodRole is the role of LZ2: incidents only where a group of the person takes part.
const prodRole = "load_lz2_team"

var prodData bool

// UseProdData points LZ1, LZ2 and LZ4 at the incidents in Prod size.
func UseProdData() { prodData = true }

// prodScenarios replaces the scenarios that have a variant in Prod size.
func prodScenarios(all []Scenario) []Scenario {
	if !prodData {
		return all
	}
	for i, s := range all {
		switch s.ID {
		case "LZ1":
			all[i].Title, all[i].Prepare = "Eine zufällige Störung von 18 Mio. lesen (Prod-Größe)", prepareReadProd
		case "LZ2":
			all[i].Title, all[i].Prepare = "Warteschlange „Störungen meines Teams“, 50 Zeilen, Rolle nur für beteiligte Gruppen (Prod-Größe)", prepareListProd
		case "LZ4":
			all[i].Title, all[i].Prepare = "Eine Störung anlegen, mit den Regeln und Fristen der Pakete (Prod-Größe)", prepareCreateProd
			all[i].Cleanup = cleanupCreatedProd
		case "LZ18":
			all[i].Call, all[i].WaitsFor = prepareDashboard, ""
		case "LZ18w":
			all[i].Prepare, all[i].WaitsFor = prepareWidget, ""
		case "LZ19":
			all[i].Call, all[i].WaitsFor = prepareStored, ""
		case "LZ20":
			all[i].Call, all[i].WaitsFor = prepareRefresh, ""
		case "LZ21a":
			all[i].Prepare, all[i].Beside, all[i].WaitsFor = prepareReadProd, besideDashboards, ""
		case "LZ21b":
			all[i].Prepare, all[i].Beside, all[i].WaitsFor = prepareListProd, besideDashboards, ""
		case "LZ21c":
			all[i].Prepare, all[i].Beside, all[i].WaitsFor = prepareCreateProd, besideDashboards, ""
			all[i].Cleanup = cleanupCreatedProd
		}
	}
	return all
}

// prepareReadProd picks 1,000 incidents at random by their running number.
func prepareReadProd(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	var count struct {
		Rows []map[string]float64 `json:"rows"`
	}
	if _, err := s.post(ctx, client, base+"/api/v1/records/"+incidentType+":query",
		map[string]any{"aggregate": map[string]any{"n": map[string]any{"count": "*"}}}, &count); err != nil {
		return nil, err
	}
	if len(count.Rows) == 0 || count.Rows[0]["n"] < 1 {
		return nil, errors.New("LZ1: no incidents; run test/load fill first")
	}
	n := int64(count.Rows[0]["n"])
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // the same sample on every run, no secret
	var ids []string
	for len(ids) < 1000 {
		nums := make([]int64, 500)
		for i := range nums {
			nums[i] = 1 + rng.Int64N(n)
		}
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if _, err := s.post(ctx, client, base+"/api/v1/records/"+incidentType+":query", map[string]any{
			"where": map[string]any{"field": "number", "in": nums}, "fields": []string{"id"}, "limit": 500,
		}, &page); err != nil {
			return nil, err
		}
		if len(page.Items) == 0 {
			return nil, errors.New("LZ1: no incident found by its number")
		}
		for _, it := range page.Items {
			ids = append(ids, it.ID)
		}
	}
	var next atomic.Uint64
	return func(ctx context.Context, base string) (*http.Request, error) {
		id := ids[next.Add(1)%uint64(len(ids))]
		return s.get("/api/v1/records/"+incidentType+"/"+id)(ctx, base)
	}, nil
}

// prepareListProd puts the load account's person in the team, gives it the role that reads
// only incidents a group of the person takes part in, and narrows a session to that role.
func prepareListProd(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	admin, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	var me struct {
		PersonID string `json:"person_id"`
	}
	if err := admin.getJSON(ctx, client, base+"/api/v1/auth/me", &me); err != nil {
		return nil, err
	}
	var team struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if _, err := admin.post(ctx, client, base+"/api/v1/records/core.group:query", map[string]any{
		"where": map[string]any{"field": "name", "eq": ProdTeam}, "fields": []string{"id"},
	}, &team); err != nil {
		return nil, err
	}
	if len(team.Items) == 0 {
		return nil, fmt.Errorf("LZ2: no group %q; run test/load fill first", ProdTeam)
	}
	again := func(_ http.Header, err error) error {
		// The setup may run on an instance that has it already.
		if err != nil && !strings.Contains(err.Error(), "409") && !strings.Contains(err.Error(), "422") {
			return err
		}
		return nil
	}
	if err := again(admin.post(ctx, client, base+"/api/v1/records/core.group_member",
		map[string]any{"group": team.Items[0].ID, "person": me.PersonID}, nil)); err != nil {
		return nil, err
	}
	if err := again(admin.post(ctx, client, base+"/api/v1/roles",
		map[string]any{"name": prodRole, "label": map[string]string{"de": "Lasttest Team"}}, nil)); err != nil {
		return nil, err
	}
	if err := admin.put(ctx, client, base+"/api/v1/roles/"+prodRole+"/permissions",
		map[string]any{"type_name": incidentType, "action": "read", "scope": "involved"}); err != nil {
		return nil, err
	}
	if err := again(admin.post(ctx, client, base+"/api/v1/assignments",
		map[string]any{"role": prodRole, "person_id": me.PersonID, "org_path": "root"}, nil)); err != nil {
		return nil, err
	}
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	if _, err := s.post(ctx, client, base+"/api/v1/auth/context", map[string]any{"role": prodRole, "org_path": "root"}, nil); err != nil {
		return nil, err
	}
	body := map[string]any{
		"where":  map[string]any{"field": "state", "in": []string{"new", "in_progress", "waiting"}},
		"fields": []string{"number", "title", "state", "priority", "sla_due_at"}, "sort": []string{"-created_at"}, "limit": 50,
	}
	return s.postFunc(base+"/api/v1/records/"+incidentType+":query", body), nil
}

// prepareCreateProd creates incidents: the rules of the packages and two clocks each.
func prepareCreateProd(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	return s.postFunc(base+"/api/v1/records/"+incidentType,
		map[string]any{"title": prodCreateTitle, "impact": "medium", "urgency": "low"}), nil
}

// prodCreateTitle is the title of the incidents LZ4 creates; only they go again.
const prodCreateTitle = "LZ4 Drucker fällt aus"

// cleanupCreatedProd removes the incidents of LZ4 for good: into the trash with :batch, then
// purged one by one in parallel. Only records without a source and with the title of the
// scenario, never the data of the fill.
func cleanupCreatedProd(ctx context.Context, client *http.Client, base string) (int, error) {
	if Database != "" {
		return cleanupCreatedSQL(ctx, Database)
	}
	s, err := login(ctx, client, base)
	if err != nil {
		return 0, err
	}
	total := 0
	for {
		var page struct {
			Items []struct {
				ID      string `json:"id"`
				Version int64  `json:"version"`
			} `json:"items"`
		}
		if _, err := s.post(ctx, client, base+"/api/v1/records/"+incidentType+":query", map[string]any{
			"where": map[string]any{"and": []any{
				map[string]any{"field": "title", "eq": prodCreateTitle},
				map[string]any{"field": "source", "null": true},
			}},
			"fields": []string{"id", "version"}, "limit": 500,
		}, &page); err != nil {
			return total, err
		}
		if len(page.Items) == 0 {
			return total, nil
		}
		rows := make([]map[string]any, len(page.Items))
		for i, it := range page.Items {
			rows[i] = map[string]any{"id": it.ID, "version": it.Version, "trash": true}
		}
		var out struct {
			Results []struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"results"`
		}
		if _, err := s.post(ctx, client, base+"/api/v1/records/"+incidentType+":batch", map[string]any{"rows": rows}, &out); err != nil {
			return total, err
		}
		for _, r := range out.Results {
			if r.Status != "ok" {
				return total, fmt.Errorf("trash: %s %s", r.Status, r.Message)
			}
		}
		ids := make(chan string)
		var wg sync.WaitGroup
		var failed atomic.Value
		for range 16 {
			wg.Go(func() {
				for id := range ids {
					if _, err := s.post(ctx, client, base+"/api/v1/records/"+incidentType+"/"+id+":purge", map[string]any{}, nil); err != nil {
						failed.Store(err)
					}
				}
			})
		}
		for _, it := range page.Items {
			ids <- it.ID
		}
		close(ids)
		wg.Wait()
		if err, ok := failed.Load().(error); ok {
			return total, fmt.Errorf("purge: %w", err)
		}
		total += len(page.Items)
	}
}
