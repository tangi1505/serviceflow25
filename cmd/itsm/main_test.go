// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"bytes"
	"strings"
	"testing"

	"group24.de/itsm/internal/platform"
)

func noenv(string) string { return "" }

func TestRunWithoutCommandPrintsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, noenv, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "usage: itsm") {
		t.Fatalf("usage missing, got %q", errOut.String())
	}
}

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, noenv, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := strings.TrimSpace(out.String()); got != platform.Version {
		t.Fatalf("version = %q, want %q", got, platform.Version)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"nope"}, noenv, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestServeNeedsConfiguration(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"serve"}, noenv, &out, &errOut); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "ITSM_DATABASE_URL") {
		t.Fatalf("error should name the missing variable, got %q", errOut.String())
	}
}

func TestServeRejectsUnknownRole(t *testing.T) {
	env := func(k string) string {
		switch k {
		case "ITSM_DATABASE_URL":
			return "postgres://x"
		case "ITSM_VALKEY_ADDR":
			return "x:6379"
		}
		return ""
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"serve", "--role=boss"}, env, &out, &errOut); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "role") {
		t.Fatalf("error should mention the role, got %q", errOut.String())
	}
}
