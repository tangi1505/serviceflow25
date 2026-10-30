// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package gate decides whether a measurement passes (Hauptplan E32, E63, E64, E69):
//   - quick stage: the new build is measured alternately with the approved baseline build on the
//     same machine at the same time; no scenario may be more than 10 % slower than the baseline;
//   - full stage: every measurable target LZ1 to LZ13 must be met;
//   - both: no errors;
//   - a throughput run (LZ5) must reach its target in both stages, and in the quick stage it may
//     not be more than 10 % below the baseline when the baseline can run it (PL32).
package gate

import (
	"fmt"
	"sort"

	"group24.de/itsm/test/load/internal/machine"
	"group24.de/itsm/test/load/internal/measure"
	"group24.de/itsm/test/load/internal/scenario"
)

// Report is what a run writes and what the baseline file holds.
type Report struct {
	Stage   string          `json:"stage"`
	Machine machine.Machine `json:"machine"`
	// Load is the 1-minute load average when the run started.
	Load float64 `json:"load1"`
	// Results of the new build.
	Results map[string]measure.Result `json:"results"`
	// Baseline holds the results of the baseline build, measured alternately with Results;
	// empty when no baseline build was available.
	Baseline map[string]measure.Result `json:"baseline,omitempty"`
	// BaselineSHA256 identifies the baseline binary that was measured.
	BaselineSHA256 string            `json:"baseline_sha256,omitempty"`
	Waiting        map[string]string `json:"waiting,omitempty"`
	// BinarySHA256, Note and ApprovedBy are set in the baseline file (E64).
	BinarySHA256 string `json:"binary_sha256,omitempty"`
	ApprovedBy   string `json:"approved_by,omitempty"`
	Note         string `json:"note,omitempty"`
}

// Rules hold the tolerances of the quick stage.
type Rules struct {
	// MaxSlowdown is the allowed relative slowdown of p95, 0.10 for 10 %.
	MaxSlowdown float64
	// FloorMs ignores slowdowns smaller than this many milliseconds (measurement noise).
	FloorMs float64
}

// Finding is one line of the verdict.
type Finding struct {
	ID   string
	Fail bool
	Text string
}

// Quick compares the new build with the baseline build measured in the same run; throughput
// runs are judged by throughput().
func Quick(cur Report, r Rules, all []scenario.Scenario) []Finding {
	var out []Finding
	cur, out = throughput(cur, r, all)
	if len(cur.Baseline) == 0 {
		out = append(out, Finding{ID: "-", Text: "keine Basislinie gemessen (kein freigegebenes Binary im Volume): nur Werte, kein Vergleich"})
		for _, id := range keys(cur.Results) {
			c := cur.Results[id]
			out = append(out, errorFindings(id, c)...)
			out = append(out, Finding{ID: id, Text: fmt.Sprintf("p95 %.2f ms, %.0f Anfragen/s", c.P95ms, c.RPS)})
		}
		return out
	}
	for _, id := range keys(cur.Baseline) {
		b := cur.Baseline[id]
		c, ok := cur.Results[id]
		if !ok {
			out = append(out, Finding{ID: id, Fail: true, Text: "an der Basislinie gemessen, am neuen Stand nicht"})
			continue
		}
		out = append(out, errorFindings(id, c)...)
		if b.Unavailable != "" || (b.Requests > 0 && b.Errors == b.Requests) {
			// The baseline build does not know this scenario yet (new endpoint): no comparison.
			out = append(out, Finding{ID: id, Text: fmt.Sprintf("p95 %.2f ms, %.0f Anfragen/s; die Basislinie beherrscht das Szenario noch nicht, kein Vergleich", c.P95ms, c.RPS)})
			continue
		}
		out = append(out, errorFindings(id+" (Basislinie)", b)...)
		limit := b.P95ms * (1 + r.MaxSlowdown)
		switch {
		case c.P95ms > limit && c.P95ms-b.P95ms > r.FloorMs:
			out = append(out, Finding{ID: id, Fail: true, Text: fmt.Sprintf(
				"p95 %.2f ms statt %.2f ms an der Basislinie: %.0f %% langsamer, erlaubt sind %.0f %%",
				c.P95ms, b.P95ms, (c.P95ms/b.P95ms-1)*100, r.MaxSlowdown*100)})
		default:
			out = append(out, Finding{ID: id, Text: fmt.Sprintf("p95 %.2f ms, Basislinie %.2f ms im selben Lauf (%+.0f %%)",
				c.P95ms, b.P95ms, (c.P95ms/b.P95ms-1)*100)})
		}
	}
	for _, id := range keys(cur.Results) {
		if _, ok := cur.Baseline[id]; !ok {
			out = append(out, errorFindings(id, cur.Results[id])...)
			out = append(out, Finding{ID: id, Text: "neu gemessen, die Basislinie kennt es noch nicht"})
		}
	}
	return out
}

// throughput judges the throughput runs (results with Rows) and returns the report without
// them, for the comparison of latencies.
func throughput(cur Report, r Rules, all []scenario.Scenario) (Report, []Finding) {
	var out []Finding
	targets := map[string]float64{}
	for _, s := range all {
		targets[s.ID] = s.Target
	}
	rest := cur
	rest.Results = map[string]measure.Result{}
	rest.Baseline = nil
	if cur.Baseline != nil {
		rest.Baseline = map[string]measure.Result{}
		for id, b := range cur.Baseline {
			if c, ok := cur.Results[id]; !ok || c.Rows == 0 {
				rest.Baseline[id] = b
			}
		}
	}
	for _, id := range keys(cur.Results) {
		c := cur.Results[id]
		b, hasBase := cur.Baseline[id]
		if c.Rows == 0 {
			rest.Results[id] = c
			continue
		}
		target := targets[id]
		switch {
		case c.RPS < target:
			out = append(out, Finding{ID: id, Fail: true, Text: fmt.Sprintf("%d Zeilen, %.0f Zeilen/s, Ziel ≥ %.0f", c.Rows, c.RPS, target)})
		case hasBase && b.Unavailable == "" && b.RPS > 0 && c.RPS < b.RPS*(1-r.MaxSlowdown):
			out = append(out, Finding{ID: id, Fail: true, Text: fmt.Sprintf("%.0f Zeilen/s statt %.0f an der Basislinie: %.0f %% weniger, erlaubt sind %.0f %%",
				c.RPS, b.RPS, (1-c.RPS/b.RPS)*100, r.MaxSlowdown*100)})
		case hasBase && b.Unavailable == "" && b.RPS > 0:
			out = append(out, Finding{ID: id, Text: fmt.Sprintf("%.0f Zeilen/s, Basislinie %.0f im selben Lauf, Ziel ≥ %.0f", c.RPS, b.RPS, target)})
		default:
			out = append(out, Finding{ID: id, Text: fmt.Sprintf("%d Zeilen, %.0f Zeilen/s, Ziel ≥ %.0f; die Basislinie beherrscht das Szenario noch nicht", c.Rows, c.RPS, target)})
		}
	}
	return rest, out
}

// Full checks the absolute targets of a run.
func Full(cur Report, all []scenario.Scenario) []Finding {
	var out []Finding
	for _, s := range all {
		c, ok := cur.Results[s.ID]
		if !ok {
			if !s.Ready() {
				out = append(out, Finding{ID: s.ID, Text: "noch nicht messbar, wartet auf " + s.WaitsFor})
			} else {
				out = append(out, Finding{ID: s.ID, Fail: true, Text: "messbar, aber nicht gemessen"})
			}
			continue
		}
		out = append(out, errorFindings(s.ID, c)...)
		switch s.Kind {
		case scenario.P95:
			out = append(out, Finding{ID: s.ID, Fail: c.P95ms > s.Target, Text: fmt.Sprintf("p95 %.2f ms, Ziel ≤ %.0f ms", c.P95ms, s.Target)})
		case scenario.RPS:
			unit := "Anfragen/s"
			if c.Rows > 0 {
				unit = "Zeilen/s"
			}
			out = append(out, Finding{ID: s.ID, Fail: c.RPS < s.Target, Text: fmt.Sprintf("%.0f %s, Ziel ≥ %.0f", c.RPS, unit, s.Target)})
		default:
			out = append(out, Finding{ID: s.ID, Text: fmt.Sprintf("p95 %.2f ms, %.0f Anfragen/s, ohne festes Ziel", c.P95ms, c.RPS)})
		}
	}
	return out
}

// Failed reports whether any finding fails.
func Failed(f []Finding) bool {
	for _, x := range f {
		if x.Fail {
			return true
		}
	}
	return false
}

func errorFindings(id string, r measure.Result) []Finding {
	if r.Errors == 0 {
		return nil
	}
	return []Finding{{ID: id, Fail: true, Text: fmt.Sprintf("%d von %d Anfragen fehlgeschlagen", r.Errors, r.Requests)}}
}

func keys(m map[string]measure.Result) []string {
	k := make([]string, 0, len(m))
	for id := range m {
		k = append(k, id)
	}
	sort.Strings(k)
	return k
}
