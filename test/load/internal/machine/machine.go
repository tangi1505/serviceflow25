// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package machine records the hardware a measurement ran on (Hauptplan E50): results are
// only comparable on the same machine.
package machine

import (
	"bufio"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Machine describes the measuring machine.
type Machine struct {
	CPU       string  `json:"cpu"`
	Cores     int     `json:"cores"`
	MemoryGB  float64 `json:"memory_gb"`
	Kernel    string  `json:"kernel"`
	GoVersion string  `json:"go_version"`
}

// Detect reads the machine from /proc; fields it cannot read stay empty.
func Detect() Machine {
	m := Machine{Cores: runtime.NumCPU(), GoVersion: runtime.Version()}
	m.CPU = firstValue("/proc/cpuinfo", "model name")
	if kb := firstValue("/proc/meminfo", "MemTotal"); kb != "" {
		if n, err := strconv.ParseFloat(strings.Fields(kb)[0], 64); err == nil {
			m.MemoryGB = math.Round(n/1024/1024*10) / 10
		}
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		m.Kernel = strings.TrimSpace(string(b))
	}
	return m
}

// SameHardware reports whether two measurements are comparable.
func (m Machine) SameHardware(o Machine) bool {
	return m.CPU == o.CPU && m.Cores == o.Cores && math.Abs(m.MemoryGB-o.MemoryGB) < 0.5
}

func firstValue(path, key string) string {
	f, err := os.Open(path) //nolint:gosec // G304: fixed paths below /proc, not user input
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
