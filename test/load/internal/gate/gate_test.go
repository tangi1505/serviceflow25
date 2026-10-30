// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package gate

import (
	"strings"
	"testing"

	"group24.de/itsm/test/load/internal/measure"
	"group24.de/itsm/test/load/internal/scenario"
)

var rules = Rules{MaxSlowdown: 0.10, FloorMs: 0.1}

func ab(newP95, baseP95 map[string]float64) Report {
	r := Report{Results: map[string]measure.Result{}, Baseline: map[string]measure.Result{}}
	for id, v := range newP95 {
		r.Results[id] = measure.Result{Requests: 100, P95ms: v}
	}
	for id, v := range baseP95 {
		r.Baseline[id] = measure.Result{Requests: 100, P95ms: v}
	}
	return r
}

func TestQuickPassesWithinTolerance(t *testing.T) {
	if f := Quick(ab(map[string]float64{"LP2": 10.9}, map[string]float64{"LP2": 10}), rules, nil); Failed(f) {
		t.Fatalf("9 %% slower must pass: %+v", f)
	}
}

func TestQuickFailsAboveTolerance(t *testing.T) {
	if f := Quick(ab(map[string]float64{"LP2": 11.5}, map[string]float64{"LP2": 10}), rules, nil); !Failed(f) {
		t.Fatalf("15 %% slower must fail: %+v", f)
	}
}

func TestQuickIgnoresNoiseBelowFloor(t *testing.T) {
	if f := Quick(ab(map[string]float64{"LP1": 0.58}, map[string]float64{"LP1": 0.50}), rules, nil); Failed(f) {
		t.Fatalf("0.08 ms more is below the floor: %+v", f)
	}
}

func TestQuickWithoutBaselineOnlyReports(t *testing.T) {
	r := ab(map[string]float64{"LP1": 5}, nil)
	f := Quick(r, rules, nil)
	if Failed(f) || !strings.Contains(f[0].Text, "keine Basislinie") {
		t.Fatalf("without baseline the gate reports, it does not fail: %+v", f)
	}
}

func TestQuickFailsOnErrorsAndMissingScenario(t *testing.T) {
	r := ab(map[string]float64{"LP1": 1}, map[string]float64{"LP1": 1})
	r.Results["LP1"] = measure.Result{Requests: 10, Errors: 1, P95ms: 1}
	if !Failed(Quick(r, rules, nil)) {
		t.Error("errors must fail")
	}
	if !Failed(Quick(ab(map[string]float64{"LP1": 1}, map[string]float64{"LP1": 1, "LP2": 1}), rules, nil)) {
		t.Error("a scenario measured only at the baseline must fail")
	}
}

func TestQuickSkipsScenarioTheBaselineCannotServe(t *testing.T) {
	r := ab(map[string]float64{"LZ1": 2}, map[string]float64{"LZ1": 0})
	r.Baseline["LZ1"] = measure.Result{Requests: 50, Errors: 50}
	f := Quick(r, rules, nil)
	if Failed(f) || !strings.Contains(f[0].Text, "beherrscht das Szenario noch nicht") {
		t.Fatalf("an all-errors baseline means no comparison: %+v", f)
	}
}

func TestFullJudgesTargetsAndReportsWaiting(t *testing.T) {
	all := []scenario.Scenario{
		{ID: "LZ1", Kind: scenario.P95, Target: 50, Request: scenario.All()[0].Request},
		{ID: "LZ11", Kind: scenario.RPS, Target: 2000, Request: scenario.All()[0].Request},
		{ID: "LZ3", Kind: scenario.P95, Target: 300, WaitsFor: "PL26"},
	}
	cur := Report{Results: map[string]measure.Result{
		"LZ1":  {Requests: 10, P95ms: 40},
		"LZ11": {Requests: 10, RPS: 1500},
	}}
	f := Full(cur, all)
	if !Failed(f) {
		t.Fatal("LZ11 below target must fail")
	}
	var waiting bool
	for _, x := range f {
		if x.ID == "LZ3" && strings.Contains(x.Text, "wartet auf PL26") && !x.Fail {
			waiting = true
		}
		if x.ID == "LZ1" && x.Fail {
			t.Error("LZ1 meets its target")
		}
	}
	if !waiting {
		t.Errorf("LZ3 must be reported as waiting: %+v", f)
	}
}

// A throughput run (LZ5, PL32) is judged by its target, and against the baseline when the
// baseline can run it; the latency comparison does not see it.
func TestQuickJudgesThroughput(t *testing.T) {
	all := []scenario.Scenario{{ID: "LZ5", Kind: scenario.RPS, Target: 5000}}
	run := func(rps, base float64, unavailable string) []Finding {
		r := Report{Results: map[string]measure.Result{"LZ5": {Requests: 1, Rows: 20000, RPS: rps}}, Baseline: map[string]measure.Result{}}
		if base > 0 || unavailable != "" {
			r.Baseline["LZ5"] = measure.Result{Requests: 1, Rows: 20000, RPS: base, Unavailable: unavailable}
		}
		return Quick(r, rules, all)
	}
	if f := run(12000, 0, "no bulk endpoint"); Failed(f) || !strings.Contains(f[0].Text, "beherrscht") {
		t.Fatalf("new endpoint: %+v", f)
	}
	if f := run(4000, 0, "no bulk endpoint"); !Failed(f) {
		t.Fatalf("below the target: %+v", f)
	}
	if f := run(10000, 12000, ""); !Failed(f) {
		t.Fatalf("17 %% below the baseline: %+v", f)
	}
	if f := run(11500, 12000, ""); Failed(f) {
		t.Fatalf("4 %% below the baseline: %+v", f)
	}
	if f := Full(Report{Results: map[string]measure.Result{"LZ5": {Requests: 1, Rows: 100000, RPS: 4999}}}, all); !Failed(f) || !strings.Contains(f[0].Text, "Zeilen/s") {
		t.Fatalf("full stage: %+v", f)
	}
}
