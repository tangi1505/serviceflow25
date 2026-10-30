// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"strings"
	"testing"
)

// Wrong rollups of the probe package are refused, each with its reason (WF13).
func TestRollupValidation(t *testing.T) {
	core, err := Load("../../packages/core")
	if err != nil {
		t.Fatal(err)
	}
	installed := map[string]bool{}
	for _, ct := range core.Types {
		installed[ct.QualifiedName()] = true
	}
	cases := []struct {
		rollup Rollup
		want   string
	}{
		{Rollup{From: "core.person", Via: "parent", Op: RollupCount}, "must be a type of this package"},
		{Rollup{From: "probe.nothing", Via: "parent", Op: RollupCount}, "no such type in this package"},
		{Rollup{From: "probe.task", Via: "title", Op: RollupCount}, "must be a reference field of probe.task"},
		{Rollup{From: "probe.task", Via: "parent", Op: RollupSum, Field: "title"}, "needs a number field"},
		{Rollup{From: "probe.task", Via: "parent", Op: RollupCount, Field: "minutes"}, "count takes no field"},
		{Rollup{From: "probe.task", Via: "parent", Op: "avg"}, "must be sum or count"},
		{Rollup{From: "probe.task", Via: "parent", Op: RollupCount, Where: "record.state =="}, "rollup where"},
	}
	for _, c := range cases {
		p, err := Load("../../packages/probe")
		if err != nil {
			t.Fatal(err)
		}
		for i := range p.Types {
			if p.Types[i].Name != "probe" {
				continue
			}
			for j := range p.Types[i].Fields {
				if p.Types[i].Fields[j].Name == "tasks_open" {
					r := c.rollup
					p.Types[i].Fields[j].Rollup = &r
				}
			}
		}
		err = Validate(p, installed)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: want %q, got %v", c.rollup, c.want, err)
		}
	}
	// The rollups are kept by the core: computed, not writable.
	p, err := Load("../../packages/probe")
	if err != nil {
		t.Fatal(err)
	}
	ty, _ := p.Type("probe")
	if f, ok := ty.Field("task_minutes"); !ok || !f.Computed || !f.Calculated() {
		t.Fatalf("task_minutes: %+v", f)
	}
	// The counted type knows what counts it, so that a write need not load every type.
	if task, _ := p.Type("task"); len(task.CountedBy) != 2 || task.CountedBy[0] != "probe.probe.task_minutes" {
		t.Fatalf("counted by: %v", task.CountedBy)
	}
	if len(ty.CountedBy) != 0 {
		t.Fatalf("probe is counted by nothing: %v", ty.CountedBy)
	}
}
