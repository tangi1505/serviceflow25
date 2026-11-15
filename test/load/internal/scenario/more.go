// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- LZ14 to LZ17: the endpoints since 2026-10-28 (H14, Hauptplan E128) ---
//
// LZ14 exports 10,000 rows as CSV and LZ15 changes 1,000 chosen rows with :batch, each a run
// measured as a whole against a target in seconds. LZ16 is the list of a saved search with a
// filter on a role ("my groups"), LZ17 the timeline of a record with 1,000 entries, both by
// the p95 of single requests. What a run needs is made once per target; the first run of a
// target then takes longer, the best run counts.

const (
	exportRows = 10_000
	batchRows  = 1_000
	// timelineEntries is the length of the LZ17 timeline.
	timelineEntries = 1_000
)

// once keeps what was prepared per scenario and target for the runs that follow.
var once sync.Map

type prepared struct {
	sync.Mutex
	done bool
	ids  []string
	vers []int64
}

func preparedFor(id, base string) *prepared {
	p, _ := once.LoadOrStore(id+" "+base, &prepared{})
	return p.(*prepared)
}

// post sends a JSON body and decodes the answer into v (nil: discard it); it returns the
// headers of the answer.
func (s session) post(ctx context.Context, client *http.Client, url string, body, v any) (http.Header, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	s.apply(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return resp.Header, fmt.Errorf("POST %s: %s: %s", url, resp.Status, strings.TrimSpace(string(msg)))
	}
	if v == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return resp.Header, err
	}
	return resp.Header, json.NewDecoder(resp.Body).Decode(v)
}

// probes makes sure n probe records under source exist, titled prefix and their number.
func (s session) probes(ctx context.Context, client *http.Client, base, source, prefix string, n int) error {
	body := ndjson(n, func(i int) map[string]any {
		return map[string]any{"external_key": fmt.Sprintf("%s-%06d", source, i), "title": fmt.Sprintf("%s %06d", prefix, i), "kind": "fault", "priority": i%5 + 1}
	})
	_, err := s.bulkStream(ctx, client, base, probeType, source, body)
	return err
}

// exportRun is LZ14: 10,000 probe records once, then one CSV export of them per run.
func exportRun(ctx context.Context, client *http.Client, base string, _ int) (int, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return 0, err
	}
	p := preparedFor("LZ14", base)
	p.Lock()
	if !p.done {
		if err := s.probes(ctx, client, base, "load-lz14", "LZ14 Export", exportRows); err != nil {
			p.Unlock()
			return 0, err
		}
		p.done = true
	}
	p.Unlock()
	hdr, err := s.post(ctx, client, base+"/api/v1/records/"+probeType+":export", map[string]any{
		"format": "csv", "where": map[string]any{"field": "title", "starts": "LZ14 Export"},
		"columns": []string{"title", "kind", "priority", "state", "created_at"}, "sort": "title",
	}, nil)
	if err != nil {
		return 0, err
	}
	n, _ := strconv.Atoi(hdr.Get("ITSM-Rows"))
	if n != exportRows {
		return n, fmt.Errorf("export: %d rows, want %d", n, exportRows)
	}
	return n, nil
}

// batchRun is LZ15: 1,000 probe records once, then one :batch per run that sets a new value
// in each, at the versions the run before left.
func batchRun(ctx context.Context, client *http.Client, base string, _ int) (int, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return 0, err
	}
	p := preparedFor("LZ15", base)
	p.Lock()
	defer p.Unlock()
	// Records of its own per target (source, keys and titles): the A/B of the CI runs both
	// builds on one database, and the versions one build keeps would be stale after the other
	// wrote the same records.
	tag := fmt.Sprintf("%08x", fnv32(base))
	prefix := "LZ15 Massenaktion " + tag
	if !p.done {
		if err := s.probes(ctx, client, base, "load-lz15-"+tag, prefix, batchRows); err != nil {
			return 0, err
		}
		for after := ""; ; {
			var page struct {
				Items []struct {
					ID      string `json:"id"`
					Version int64  `json:"version"`
				} `json:"items"`
				Next string `json:"next"`
			}
			q := map[string]any{"where": map[string]any{"field": "title", "starts": prefix},
				"fields": []string{"id", "version"}, "sort": []string{"title"}, "limit": 500}
			if after != "" {
				q["after"] = after
			}
			if _, err := s.post(ctx, client, base+"/api/v1/records/"+probeType+":query", q, &page); err != nil {
				return 0, err
			}
			for _, it := range page.Items {
				p.ids, p.vers = append(p.ids, it.ID), append(p.vers, it.Version)
			}
			if page.Next == "" {
				break
			}
			after = page.Next
		}
		if len(p.ids) != batchRows {
			return 0, fmt.Errorf("batch: %d records, want %d", len(p.ids), batchRows)
		}
		p.done = true
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	rows := make([]map[string]any, len(p.ids))
	for i := range p.ids {
		rows[i] = map[string]any{"id": p.ids[i], "version": p.vers[i], "set": map[string]any{"tag_no": "LZ15 " + stamp}}
	}
	var out struct {
		Results []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Version int64  `json:"version"`
			Message string `json:"message"`
		} `json:"results"`
	}
	if _, err := s.post(ctx, client, base+"/api/v1/records/"+probeType+":batch", map[string]any{"rows": rows}, &out); err != nil {
		return 0, err
	}
	n := 0
	var bad []string
	for i, r := range out.Results {
		if r.Status != "ok" {
			bad = append(bad, r.Status+" "+r.Message)
			continue
		}
		p.vers[i] = r.Version
		n++
	}
	if len(bad) > 0 {
		return n, fmt.Errorf("batch: %d rows not ok, first: %s", len(bad), bad[0])
	}
	return n, nil
}

// prepareMyGroups is LZ16: the load account's person in a group of its own, 60 probe records
// observed by that group and 60 not, and the saved search the list runs.
func prepareMyGroups(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	var me struct {
		PersonID string `json:"person_id"`
	}
	if err := s.getJSON(ctx, client, base+"/api/v1/auth/me", &me); err != nil {
		return nil, err
	}
	if me.PersonID == "" {
		return nil, errors.New("LZ16: the load account has no person")
	}
	var group struct {
		ID string `json:"id"`
	}
	name := "load LZ16 " + time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.post(ctx, client, base+"/api/v1/records/core.group", map[string]any{"name": name}, &group); err != nil {
		return nil, err
	}
	if _, err := s.post(ctx, client, base+"/api/v1/records/core.group_member", map[string]any{"group": group.ID, "person": me.PersonID}, nil); err != nil {
		return nil, err
	}
	for i := range 120 {
		rec := map[string]any{"title": fmt.Sprintf("load LZ16 %d", i), "kind": "fault", "priority": 2}
		if i%2 == 0 {
			rec["actors"] = map[string]any{"observer": []any{map[string]any{"type": "core.group", "id": group.ID}}}
		}
		if _, err := s.post(ctx, client, base+"/api/v1/records/"+probeType, rec, nil); err != nil {
			return nil, err
		}
	}
	where := map[string]any{"and": []any{
		map[string]any{"field": "actors.observer", "eq": "@my_groups"},
		map[string]any{"field": "kind", "eq": "fault"},
	}}
	raw, _ := json.Marshal(where)
	if _, err := s.post(ctx, client, base+"/api/v1/records/core.saved_search", map[string]any{
		"name": name, "target_type": probeType, "query": string(raw), "columns": "title,priority,state", "sort": "-created_at",
	}, nil); err != nil {
		return nil, err
	}
	// The membership counts from the next sign-in on.
	s, err = login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{"where": where, "sort": []string{"-created_at"}, "limit": 50, "fields": []string{"title", "priority", "state"}})
	return func(ctx context.Context, base string) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/records/"+probeType+":query", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		s.apply(req)
		return req, nil
	}, nil
}

// prepareTimeline is LZ17: one probe record with 1,000 entries; the request reads its timeline.
func prepareTimeline(ctx context.Context, client *http.Client, base string) (RequestFunc, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, err
	}
	id, err := s.createProbe(ctx, client, base, "load LZ17")
	if err != nil {
		return nil, err
	}
	for i := range timelineEntries {
		if _, err := s.post(ctx, client, base+"/api/v1/records/"+probeType+"/"+id+"/timeline",
			map[string]any{"body": fmt.Sprintf("Eintrag %d der langen Zeitleiste", i)}, nil); err != nil {
			return nil, err
		}
	}
	return s.get("/api/v1/records/" + probeType + "/" + id + "/timeline"), nil
}

// Bulk signs in and sends one NDJSON stream of typ under source, then waits for its job; it
// returns the rows written (H14: the fill in Prod size, test/load fill).
func Bulk(ctx context.Context, client *http.Client, base, typ, source string, body []byte) (int, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return 0, err
	}
	return s.bulkStream(ctx, client, base, typ, source, body)
}

// put sends a JSON body with PUT and discards the answer.
func (s session) put(ctx context.Context, client *http.Client, url string, body any) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(raw))
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
	msg, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("PUT %s: %s: %s", url, resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// postFunc is a measured POST of a fixed JSON body.
func (s session) postFunc(url string, body any) RequestFunc {
	raw, _ := json.Marshal(body)
	return func(ctx context.Context, _ string) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		s.apply(req)
		return req, nil
	}
}

// fnv32 is a short fixed tag of a text (FNV-1a).
func fnv32(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
