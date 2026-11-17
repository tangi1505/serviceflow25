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
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// --- LZ18: the dashboards in Prod size (PL66, Hauptplan E195) ---
//
// The browser opens a dashboard and asks for the data of all its widgets at once, each with
// the period the dashboard opens with (PL65): "Statistik" 7 days, "Helpdesk" all. LZ18 is one
// opening as a whole, every widget at the same time, taking turns between the two dashboards;
// LZ18w is a single widget of them while others are asked too.

// boardNames are the dashboards of the start data that LZ18 opens.
var boardNames = []string{"Statistik", "Helpdesk"}

// widgetCall is the data request of one widget with the filters of its dashboard.
type widgetCall struct {
	url     string
	filters map[string]any
}

// dashboards finds the dashboards by name and lists the data request of each of their widgets
// (text widgets need none), with the period each opens with.
func (s session) dashboards(ctx context.Context, client *http.Client, base string, now time.Time) ([][]widgetCall, error) {
	var found struct {
		Items []map[string]any `json:"items"`
	}
	where := map[string]any{"field": "name", "in": boardNames}
	if _, err := s.post(ctx, client, base+"/api/v1/records/core.dashboard:query",
		map[string]any{"where": where, "fields": []string{"name", "period"}, "limit": 10}, &found); err != nil {
		return nil, err
	}
	var boards [][]widgetCall
	for _, d := range found.Items {
		id, _ := d["id"].(string)
		filters := map[string]any{}
		switch d["period"] {
		case "today":
			y, m, day := now.Date()
			filters["from"] = time.Date(y, m, day, 0, 0, 0, 0, now.Location()).UTC().Format(time.RFC3339)
		case "days_7":
			filters["from"] = now.Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339)
		case "days_30":
			filters["from"] = now.Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
		case "year":
			filters["from"] = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).UTC().Format(time.RFC3339)
		}
		var view struct {
			Widgets []map[string]any `json:"widgets"`
		}
		if err := s.getJSON(ctx, client, base+"/api/v1/dashboards/"+id, &view); err != nil {
			return nil, err
		}
		var calls []widgetCall
		for _, w := range view.Widgets {
			if w["kind"] == "text" {
				continue
			}
			wid, _ := w["id"].(string)
			calls = append(calls, widgetCall{url: base + "/api/v1/dashboards/" + id + "/widgets/" + wid + "/data", filters: filters})
		}
		if len(calls) > 0 {
			boards = append(boards, calls)
		}
	}
	if len(boards) == 0 {
		return nil, fmt.Errorf("no dashboard %v with widgets at %s", boardNames, base)
	}
	return boards, nil
}

// prepareDashboard is LZ18: one call opens one of the dashboards, every widget at once, and
// succeeds when all of them answer.
func prepareDashboard(ctx context.Context, client *http.Client, base string) (CallFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	boards, err := s.dashboards(ctx, client, base, time.Now())
	if err != nil {
		return nil, err
	}
	var turn atomic.Uint64
	return func(ctx context.Context, client *http.Client) bool {
		calls := boards[(turn.Add(1)-1)%uint64(len(boards))]
		var wg sync.WaitGroup
		var failed atomic.Bool
		for _, c := range calls {
			wg.Go(func() {
				if _, err := s.post(ctx, client, c.url, c.filters, nil); err != nil {
					failed.Store(true)
				}
			})
		}
		wg.Wait()
		return !failed.Load()
	}, nil
}

// prepareWidget is LZ18w: one request is the data of one widget, taking turns over all widgets
// of both dashboards; the workers of the scenario ask at the same time, as the browser does.
func prepareWidget(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	boards, err := s.dashboards(ctx, client, base, time.Now())
	if err != nil {
		return nil, err
	}
	var all []RequestFunc
	for _, calls := range boards {
		for _, c := range calls {
			all = append(all, s.postFunc(c.url, c.filters))
		}
	}
	if len(all) == 0 {
		return nil, errors.New("no widget to ask")
	}
	var turn atomic.Uint64
	return func(ctx context.Context, base string) (*http.Request, error) {
		return all[(turn.Add(1)-1)%uint64(len(all))](ctx, base)
	}, nil
}
