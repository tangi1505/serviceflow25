// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package gen writes the synthetic data set as NDJSON, one file per kind (Hauptplan E65).
// Same seed, same size: byte-identical files. No real data (VS-NfD). Every line carries
// source and external_key so that the Bulk-API can reconcile it (Hauptplan E38).
package gen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"time"
)

// Size holds the amounts of one data set.
type Size struct {
	Name             string
	Persons          int
	Computers        int
	SoftwarePerPC    int
	SoftwareCatalog  int
	Hypervisors      int
	VMsPerHypervisor int
	Switches         int
	PortsPerSwitch   int
	Tickets          int
}

// Sizes known to the tool. Prod follows Hauptplan §2.2 and §2.5 (V14: 5.000 tickets a day for
// 10 years); ci is small enough for every CI run.
var Sizes = map[string]Size{
	"tiny": {Name: "tiny", Persons: 20, Computers: 10, SoftwarePerPC: 5, SoftwareCatalog: 50,
		Hypervisors: 2, VMsPerHypervisor: 3, Switches: 2, PortsPerSwitch: 4, Tickets: 30},
	"ci": {Name: "ci", Persons: 1_000, Computers: 500, SoftwarePerPC: 150, SoftwareCatalog: 2_000,
		Hypervisors: 10, VMsPerHypervisor: 40, Switches: 50, PortsPerSwitch: 48, Tickets: 10_000},
	"prod": {Name: "prod", Persons: 70_000, Computers: 50_000, SoftwarePerPC: 150, SoftwareCatalog: 2_000,
		Hypervisors: 100, VMsPerHypervisor: 40, Switches: 500, PortsPerSwitch: 48, Tickets: 18_250_000},
}

// Kinds in the order they are written; later kinds refer to earlier ones.
var Kinds = []string{"persons", "computers", "hypervisors", "switches", "tickets"}

const source = "loadgen"

// Start of the ticket history: 10 years before the reference date.
var (
	refDate   = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	histStart = refDate.AddDate(-10, 0, 0)
)

// Write writes one kind to w and returns the number of lines.
func Write(w io.Writer, kind string, s Size, seed uint64) (int, error) {
	bw := bufio.NewWriterSize(w, 1<<20)
	enc := json.NewEncoder(bw)
	// One generator per kind, so that kinds can be written separately and stay identical.
	rng := rand.New(rand.NewPCG(seed, uint64(kindIndex(kind)))) //nolint:gosec // G404: synthetic test data, not security
	var n int
	var err error
	switch kind {
	case "persons":
		n, err = persons(enc, rng, s)
	case "computers":
		n, err = computers(enc, rng, s)
	case "hypervisors":
		n, err = hypervisors(enc, rng, s)
	case "switches":
		n, err = switches(enc, rng, s)
	case "tickets":
		n, err = tickets(enc, rng, s)
	default:
		return 0, fmt.Errorf("gen: unknown kind %q", kind)
	}
	if err != nil {
		return n, err
	}
	return n, bw.Flush()
}

func kindIndex(kind string) int {
	for i, k := range Kinds {
		if k == kind {
			return i
		}
	}
	return -1
}

func key(prefix string, i int) string { return fmt.Sprintf("%s-%08d", prefix, i) }

var (
	firstNames = []string{"Anna", "Ben", "Clara", "David", "Eva", "Felix", "Greta", "Hannes", "Ida", "Jonas", "Klara", "Lukas", "Mia", "Noah", "Paula", "Rafael", "Sophie", "Tim"}
	lastNames  = []string{"Becker", "Fischer", "Hoffmann", "Keller", "Lang", "Meyer", "Neumann", "Richter", "Schulz", "Vogel", "Wagner", "Weber", "Wolf", "Zimmermann"}
	units      = []string{"Präsidium", "Direktion Nord", "Direktion Süd", "Direktion Ost", "Direktion West", "Zentrale Dienste", "Ausbildung", "Technik"}
	vendors    = []string{"Dell", "HP", "Lenovo", "Fujitsu", "Cisco", "Juniper", "Supermicro"}
	osNames    = []string{"Windows 11 Enterprise", "Ubuntu 24.04 LTS", "Debian 13", "Red Hat Enterprise Linux 10"}
	words      = []string{"Drucker", "Anmeldung", "Netzwerk", "Laufwerk", "Monitor", "Kennwort", "VPN", "Mail", "Kalender", "Software", "Zugang", "Freigabe", "Telefon", "Scanner", "Zertifikat", "Update", "Lizenz", "Speicher", "Fehler", "langsam", "fällt aus", "neu", "Anfrage"}
	states     = []string{"new", "assigned", "pending", "solved", "closed"}
	// stateWeights favour closed tickets, as in a 10-year history.
	stateWeights = []int{2, 3, 2, 3, 90}
)

func pick[T any](rng *rand.Rand, xs []T) T { return xs[rng.IntN(len(xs))] }

func persons(enc *json.Encoder, rng *rand.Rand, s Size) (int, error) {
	for i := range s.Persons {
		fn, ln := pick(rng, firstNames), pick(rng, lastNames)
		err := enc.Encode(map[string]any{
			"source": source, "external_key": key("person", i),
			"name":  fn + " " + ln,
			"email": fmt.Sprintf("%s.%s.%d@example.invalid", fn, ln, i),
			"unit":  pick(rng, units),
		})
		if err != nil {
			return i, err
		}
	}
	return s.Persons, nil
}

func computers(enc *json.Encoder, rng *rand.Rand, s Size) (int, error) {
	idx := make([]int, s.SoftwareCatalog)
	for i := range idx {
		idx[i] = i
	}
	for i := range s.Computers {
		// Partial Fisher-Yates: SoftwarePerPC distinct packages from the catalog.
		n := min(s.SoftwarePerPC, len(idx))
		sw := make([]map[string]any, n)
		for j := range n {
			k := j + rng.IntN(len(idx)-j)
			idx[j], idx[k] = idx[k], idx[j]
			sw[j] = map[string]any{
				"external_key": fmt.Sprintf("pkg-%04d", idx[j]),
				"name":         fmt.Sprintf("Paket %04d", idx[j]),
				"version":      fmt.Sprintf("%d.%d.%d", 1+idx[j]%9, rng.IntN(20), rng.IntN(50)),
			}
		}
		err := enc.Encode(map[string]any{
			"source": source, "external_key": key("computer", i),
			"name":     fmt.Sprintf("PC%06d", i),
			"serial":   fmt.Sprintf("SN%010d", rng.Int64N(1e10)),
			"vendor":   pick(rng, vendors),
			"os":       pick(rng, osNames),
			"user":     key("person", rng.IntN(max(1, s.Persons))),
			"software": sw,
		})
		if err != nil {
			return i, err
		}
	}
	return s.Computers, nil
}

func hypervisors(enc *json.Encoder, rng *rand.Rand, s Size) (int, error) {
	for i := range s.Hypervisors {
		vms := make([]map[string]any, s.VMsPerHypervisor)
		for j := range vms {
			vms[j] = map[string]any{
				"external_key": fmt.Sprintf("vm-%04d-%03d", i, j),
				"name":         fmt.Sprintf("vm%04d%03d", i, j),
				"cpus":         1 << rng.IntN(5),
				"memory_gb":    2 << rng.IntN(6),
				"powered_on":   rng.IntN(10) > 0,
			}
		}
		err := enc.Encode(map[string]any{
			"source": source, "external_key": key("hypervisor", i),
			"name": fmt.Sprintf("hv%04d", i), "vendor": pick(rng, vendors), "vms": vms,
		})
		if err != nil {
			return i, err
		}
	}
	return s.Hypervisors, nil
}

func switches(enc *json.Encoder, rng *rand.Rand, s Size) (int, error) {
	for i := range s.Switches {
		ports := make([]map[string]any, s.PortsPerSwitch)
		for j := range ports {
			ports[j] = map[string]any{
				"external_key": fmt.Sprintf("port-%04d-%02d", i, j),
				"name":         fmt.Sprintf("Gi1/0/%d", j+1),
				"mac":          fmt.Sprintf("02:00:%02x:%02x:%02x:%02x", i>>8&0xff, i&0xff, j, rng.IntN(256)),
				"up":           rng.IntN(4) > 0,
			}
		}
		err := enc.Encode(map[string]any{
			"source": source, "external_key": key("switch", i),
			"name": fmt.Sprintf("sw%04d", i), "vendor": pick(rng, vendors), "ports": ports,
		})
		if err != nil {
			return i, err
		}
	}
	return s.Switches, nil
}

func tickets(enc *json.Encoder, rng *rand.Rand, s Size) (int, error) {
	span := refDate.Sub(histStart)
	total := 0
	for _, w := range stateWeights {
		total += w
	}
	for i := range s.Tickets {
		r := rng.IntN(total)
		state := states[len(states)-1]
		for k, w := range stateWeights {
			if r < w {
				state = states[k]
				break
			}
			r -= w
		}
		// Tickets are spread evenly over the history, in order, as they would be created.
		created := histStart.Add(time.Duration(float64(span) * float64(i) / float64(max(1, s.Tickets))))
		rec := map[string]any{
			"source": source, "external_key": key("ticket", i),
			"title":      pick(rng, words) + " " + pick(rng, words),
			"state":      state,
			"impact":     1 + rng.IntN(5),
			"urgency":    1 + rng.IntN(5),
			"requester":  key("person", rng.IntN(max(1, s.Persons))),
			"created_at": created.Format(time.RFC3339),
		}
		if s.Computers > 0 && rng.IntN(2) == 0 {
			rec["computer"] = key("computer", rng.IntN(s.Computers))
		}
		if err := enc.Encode(rec); err != nil {
			return i, err
		}
	}
	return s.Tickets, nil
}
