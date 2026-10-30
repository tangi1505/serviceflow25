// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package gen

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"testing"
)

func generate(t *testing.T, kind string, seed uint64) ([]byte, int) {
	t.Helper()
	var buf bytes.Buffer
	n, err := Write(&buf, kind, Sizes["tiny"], seed)
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), n
}

func TestDeterministicPerSeed(t *testing.T) {
	for _, kind := range Kinds {
		a, _ := generate(t, kind, 1)
		b, _ := generate(t, kind, 1)
		c, _ := generate(t, kind, 2)
		if sha256.Sum256(a) != sha256.Sum256(b) {
			t.Errorf("%s: same seed gives different data", kind)
		}
		if sha256.Sum256(a) == sha256.Sum256(c) {
			t.Errorf("%s: different seeds give the same data", kind)
		}
	}
}

func TestLinesAndKeys(t *testing.T) {
	s := Sizes["tiny"]
	want := map[string]int{"persons": s.Persons, "computers": s.Computers, "hypervisors": s.Hypervisors, "switches": s.Switches, "tickets": s.Tickets}
	for kind, n := range want {
		data, got := generate(t, kind, 7)
		if got != n {
			t.Errorf("%s: %d lines reported, want %d", kind, got, n)
		}
		seen := map[string]bool{}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		lines := 0
		for sc.Scan() {
			var rec map[string]any
			if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
				t.Fatalf("%s: invalid JSON line: %v", kind, err)
			}
			k, _ := rec["external_key"].(string)
			if rec["source"] != "loadgen" || k == "" || seen[k] {
				t.Fatalf("%s: bad or duplicate key in %v", kind, rec)
			}
			seen[k] = true
			lines++
		}
		if lines != n {
			t.Errorf("%s: %d lines written, want %d", kind, lines, n)
		}
	}
}

func TestSoftwarePerComputerIsDistinct(t *testing.T) {
	data, _ := generate(t, "computers", 3)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var rec struct {
			Software []struct {
				Key string `json:"external_key"`
			} `json:"software"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		if len(rec.Software) != Sizes["tiny"].SoftwarePerPC {
			t.Fatalf("got %d programs", len(rec.Software))
		}
		seen := map[string]bool{}
		for _, p := range rec.Software {
			if seen[p.Key] {
				t.Fatalf("program %s twice on one computer", p.Key)
			}
			seen[p.Key] = true
		}
	}
}

func TestProdSizeMatchesPlan(t *testing.T) {
	p := Sizes["prod"]
	if p.Persons != 70_000 || p.Computers != 50_000 || p.Computers*p.SoftwarePerPC != 7_500_000 ||
		p.Hypervisors*p.VMsPerHypervisor != 4_000 || p.Tickets != 18_250_000 {
		t.Fatalf("prod size drifted from Hauptplan §2.2/§2.5: %+v", p)
	}
}
