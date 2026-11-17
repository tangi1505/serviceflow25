// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package scenario

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// --- LZ18: the dashboards in Prod size (PL66, PL67, Hauptplan E195) ---
//
// The browser opens a dashboard and asks for the data of all its widgets in one request (PL67),
// with the period the dashboard opens with (PL65): "Statistik" 7 days, "Helpdesk" all. LZ18 is
// one opening as a whole, until the last widget has come, taking turns between the two
// dashboards; LZ18w is a single widget of them (the sidebar and the tiles ask so) while others
// are asked too. LZ21 measures LZ1, LZ2 and LZ4 while 20 dashboards count over everything.

// boardNames are the dashboards of the start data that LZ18 opens.
var boardNames = []string{"Statistik", "Helpdesk"}

// board is a dashboard with the filters of the period it opens with and its widgets.
type board struct {
	name    string
	data    string
	filters map[string]any
	widgets []string
}

// boards finds the dashboards by name with the data request of each widget (text widgets need
// none) and the period each opens with.
func (s session) boards(ctx context.Context, client *http.Client, base string, now time.Time) ([]board, error) {
	var found struct {
		Items []map[string]any `json:"items"`
	}
	where := map[string]any{"field": "name", "in": boardNames}
	if _, err := s.post(ctx, client, base+"/api/v1/records/core.dashboard:query",
		map[string]any{"where": where, "fields": []string{"name", "period"}, "limit": 10}, &found); err != nil {
		return nil, err
	}
	var out []board
	for _, d := range found.Items {
		id, _ := d["id"].(string)
		name, _ := d["name"].(string)
		b := board{name: name, data: base + "/api/v1/dashboards/" + id + "/data", filters: map[string]any{}}
		switch d["period"] {
		case "today":
			y, m, day := now.Date()
			b.filters["from"] = time.Date(y, m, day, 0, 0, 0, 0, now.Location()).UTC().Format(time.RFC3339)
		case "days_7":
			b.filters["from"] = now.Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339)
		case "days_30":
			b.filters["from"] = now.Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
		case "year":
			b.filters["from"] = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).UTC().Format(time.RFC3339)
		}
		var view struct {
			Widgets []map[string]any `json:"widgets"`
		}
		if err := s.getJSON(ctx, client, base+"/api/v1/dashboards/"+id, &view); err != nil {
			return nil, err
		}
		for _, w := range view.Widgets {
			if w["kind"] == "text" {
				continue
			}
			wid, _ := w["id"].(string)
			b.widgets = append(b.widgets, base+"/api/v1/dashboards/"+id+"/widgets/"+wid+"/data")
		}
		if len(b.widgets) > 0 {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no dashboard %v with widgets at %s", boardNames, base)
	}
	return out, nil
}

// open asks for all widgets of a dashboard in one request (PL67) and reads the lines to the
// end; it fails when the answer is no 200 or a widget carries a problem. ready, if set, counts
// the widgets that came.
func (s session) open(ctx context.Context, client *http.Client, url string, filters map[string]any, ready *atomic.Int64) error {
	raw, _ := json.Marshal(filters)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	s.apply(req)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %s: %s", url, resp.Status, bytes.TrimSpace(msg))
	}
	lines := bufio.NewScanner(resp.Body)
	lines.Buffer(make([]byte, 64*1024), 4<<20)
	for lines.Scan() {
		var l struct {
			Widget  string          `json:"widget"`
			Problem json.RawMessage `json:"problem"`
		}
		if err := json.Unmarshal(lines.Bytes(), &l); err != nil {
			return err
		}
		if len(l.Problem) > 0 {
			return fmt.Errorf("widget %s: %s", l.Widget, l.Problem)
		}
		if ready != nil {
			ready.Add(1)
		}
	}
	return lines.Err()
}

// prepareDashboard is LZ18: one call opens one of the dashboards and succeeds when all its
// widgets have come.
func prepareDashboard(ctx context.Context, client *http.Client, base string) (CallFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	boards, err := s.boards(ctx, client, base, time.Now())
	if err != nil {
		return nil, err
	}
	var turn atomic.Uint64
	return func(ctx context.Context, client *http.Client) bool {
		b := boards[(turn.Add(1)-1)%uint64(len(boards))]
		return s.open(ctx, client, b.data, b.filters, nil) == nil
	}, nil
}

// prepareWidget is LZ18w: one request is the data of one widget, taking turns over all widgets
// of both dashboards; the workers of the scenario ask at the same time.
func prepareWidget(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	boards, err := s.boards(ctx, client, base, time.Now())
	if err != nil {
		return nil, err
	}
	var all []RequestFunc
	for _, b := range boards {
		for _, url := range b.widgets {
			all = append(all, s.postFunc(url, b.filters))
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

// besideBoards is how many dashboards count at the same time beside LZ21.
const besideBoards = 20

// besideDashboards is the load beside LZ21: 20 openings of "Statistik" over everything
// ("Gesamt"), each again as soon as it is done, until stop. They use a client of their own:
// the one of the measurement gives up after 10 s, an opening over everything takes longer.
func besideDashboards(ctx context.Context, _ *http.Client, base string) (func() string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	boards, err := s.boards(ctx, client, base, time.Now())
	if err != nil {
		return nil, err
	}
	url := ""
	for _, b := range boards {
		if b.name == "Statistik" {
			url = b.data
		}
	}
	if url == "" {
		return nil, errors.New("no dashboard Statistik")
	}
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	var done, failed, widgets atomic.Int64
	for range besideBoards {
		wg.Go(func() {
			for ctx.Err() == nil {
				if err := s.open(ctx, client, url, map[string]any{}, &widgets); err != nil {
					if ctx.Err() == nil {
						failed.Add(1)
						time.Sleep(100 * time.Millisecond)
					}
					continue
				}
				done.Add(1)
			}
		})
	}
	return func() string {
		cancel()
		wg.Wait()
		return fmt.Sprintf("daneben %d Widgets über alles gerechnet, %d Dashboards ganz geöffnet, %d mit Fehler", widgets.Load(), done.Load(), failed.Load())
	}, nil
}
