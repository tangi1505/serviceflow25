// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// The Prüfweg of WF13: three tasks at a record, the sum of their times and the number of open
// ones appear at the record; finishing, changing, moving, trashing and restoring a task keep
// both right; the record keeps its version, its history shows the sums. A task with a date is
// found by a query on its planned time (the calendar of UI15 reads it so). Tasks written at
// once each count the other.
func TestTasksOverHTTP(t *testing.T) {
	api := apiServer(t)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	const base = "/api/v1/records/probe.probe"
	const tasks = "/api/v1/records/probe.task"
	var made []string
	defer func() {
		for i := len(made) - 1; i >= 0; i-- {
			typ, id, _ := strings.Cut(made[i], "/")
			call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+id, "", nil)
			call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+id+":purge", "", nil)
		}
	}()
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		id := r.body["id"].(string)
		made = append(made, typeName+"/"+id)
		return id
	}
	sums := func(id string) (float64, float64, float64) {
		t.Helper()
		r := call(t, api, http.MethodGet, base+"/"+id, "", nil)
		if r.code != 200 {
			t.Fatalf("record: %d %s", r.code, r.raw)
		}
		minutes, _ := r.body["task_minutes"].(float64)
		open, _ := r.body["tasks_open"].(float64)
		version, _ := r.body["version"].(float64)
		return minutes, open, version
	}
	expect := func(what, id string, minutes, open float64) {
		t.Helper()
		if m, o, v := sums(id); m != minutes || o != open || v != 1 {
			t.Fatalf("%s: minutes %v open %v version %v, want %v %v 1", what, m, o, v, minutes, open)
		}
	}
	patch := func(id, body string) {
		t.Helper()
		v := call(t, api, http.MethodGet, tasks+"/"+id, "", nil).body["version"]
		if r := call(t, api, http.MethodPatch, tasks+"/"+id, body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)}); r.code != 200 {
			t.Fatalf("patch task: %d %s", r.code, r.raw)
		}
	}

	p1 := create("probe.probe", `{"title": "WF13 `+suffix+` Umzug"}`)
	p2 := create("probe.probe", `{"title": "WF13 `+suffix+` Rollout"}`)
	expect("no tasks", p1, 0, 0)
	group := create("core.group", `{"name": "WF13 Technik `+suffix+`"}`)
	t1 := create("probe.task", `{"title": "WF13 `+suffix+` Kabel", "parent": "`+p1+`", "minutes": 30}`)
	t2 := create("probe.task", `{"title": "WF13 `+suffix+` Rechner", "parent": "`+p1+`", "minutes": 45}`)
	t3 := create("probe.task", `{"title": "WF13 `+suffix+` Drucker", "parent": "`+p1+`", "minutes": 60,
		"planned_start": "2026-11-10T09:00:00Z", "planned_end": "2026-11-10T10:00:00Z",
		"actors": {"assigned": [{"type": "core.group", "id": "`+group+`"}]}}`)
	expect("three tasks", p1, 135, 3)

	// A sum is no field to write.
	if r := call(t, api, http.MethodPatch, base+"/"+p1, `{"task_minutes": 5}`, map[string]string{"If-Match": `"1"`}); r.code != 422 {
		t.Fatalf("write a sum: %d %s", r.code, r.raw)
	}
	// Finished, changed, moved to another record.
	if r := call(t, api, http.MethodPost, tasks+"/"+t1+":transition", `{"transition": "finish"}`, nil); r.code != 200 {
		t.Fatalf("finish: %d %s", r.code, r.raw)
	}
	expect("one finished", p1, 135, 2)
	patch(t2, `{"minutes": 50}`)
	expect("changed time", p1, 140, 2)
	patch(t3, `{"parent": "`+p2+`"}`)
	expect("moved away", p1, 80, 1)
	expect("moved here", p2, 60, 1)
	// In the trash a task does not count; restored, it does again.
	if r := call(t, api, http.MethodDelete, tasks+"/"+t2, "", nil); r.code != 204 {
		t.Fatalf("trash: %d %s", r.code, r.raw)
	}
	expect("in the trash", p1, 30, 0)
	if r := call(t, api, http.MethodPost, tasks+"/"+t2+":restore", "", nil); r.code != 204 && r.code != 200 {
		t.Fatalf("restore: %d %s", r.code, r.raw)
	}
	expect("restored", p1, 80, 1)
	// The history of the record shows the sums.
	if h := call(t, api, http.MethodGet, base+"/"+p1+"/history", "", nil); !strings.Contains(h.raw, `"task_minutes"`) || !strings.Contains(h.raw, `"tasks_open"`) {
		t.Fatalf("history: %s", h.raw)
	}

	// The calendar of UI15 finds tasks by their planned time.
	r := call(t, api, http.MethodPost, "/api/v1/records/probe.task:query", `{"where": {"and": [
		{"field": "planned_start", "gte": "2026-11-09T00:00:00Z"}, {"field": "planned_start", "lt": "2026-11-16T00:00:00Z"},
		{"field": "title", "starts": "WF13 `+suffix+`"}]}}`, nil)
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if r.code != 200 || json.Unmarshal([]byte(r.raw), &page) != nil || len(page.Items) != 1 || page.Items[0]["id"] != t3 {
		t.Fatalf("tasks of the week: %d %s", r.code, r.raw)
	}

	// Ten tasks written at once: each counts.
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := []string{}
	for i := range 10 {
		wg.Go(func() {
			r := call(t, api, http.MethodPost, tasks, fmt.Sprintf(`{"title": "WF13 %s parallel %d", "parent": "%s", "minutes": 1}`, suffix, i, p2), nil)
			mu.Lock()
			defer mu.Unlock()
			if r.code != 201 {
				errs = append(errs, r.raw)
				return
			}
			made = append(made, "probe.task/"+r.body["id"].(string))
		})
	}
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("parallel tasks: %v", errs)
	}
	expect("ten at once", p2, 70, 11)
}
