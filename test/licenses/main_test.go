// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mitText = `MIT License

Copyright (c) 2026 Somebody

Permission is hereby granted, free of charge, to any person obtaining a copy of this software
and associated documentation files (the "Software"), to deal in the Software without
restriction, including without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the
Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or
substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING
BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
`

func testPolicy(t *testing.T) policy {
	t.Helper()
	p, err := parsePolicy(policyText)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPolicyFile(t *testing.T) {
	p := testPolicy(t)
	for _, id := range []string{"MIT", "Apache-2.0", "BSD-3-Clause", "ISC", "MPL-2.0", "Unlicense"} {
		if !p.allowed[id] {
			t.Errorf("%s not allowed", id)
		}
	}
	// Copyleft would rule out the commercial license (Hauptplan E15, V4).
	for _, id := range []string{"GPL-3.0", "GPL-3.0-only", "LGPL-2.1", "AGPL-3.0", "AGPL-3.0-only"} {
		if p.allowed[id] {
			t.Errorf("%s allowed", id)
		}
	}
	for _, bad := range []string{"allow", "allow MIT Apache-2.0", "except x | MIT | 2026-10-28", "except x | | 2026-10-28 | why", "permit MIT"} {
		if _, err := parsePolicy(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestAllows(t *testing.T) {
	p := testPolicy(t)
	for expr, want := range map[string]bool{
		"MIT":                              true,
		"(MIT OR CC0-1.0)":                 true,
		"GPL-3.0 OR MIT":                   true,
		"Apache-2.0 AND MIT":               true,
		"MIT AND GPL-3.0":                  false,
		"(MIT OR GPL-3.0) AND LGPL-2.1":    false,
		"Apache-2.0 WITH LLVM-exception":   true,
		"GPL-2.0 WITH Classpath-exception": false,
		"":                                 false,
		"MIT OR":                           false,
		"(MIT":                             false,
		"SEE LICENSE IN LICENSE.md":        false,
	} {
		if got := p.allows(expr); got != want {
			t.Errorf("%q: %v, want %v", expr, got, want)
		}
	}
}

func TestCheck(t *testing.T) {
	p, err := parsePolicy(policyText + `
except legacy-lib | GPL-3.0 | 2026-10-28 | test
except changed-lib | MIT | 2026-10-28 | test
except gone-lib | GPL-3.0 | 2026-10-28 | test
`)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(p.check([]dep{
		{Kind: "go", Name: "fine", Version: "v1", License: "MIT"},
		{Kind: "web", Name: "copyleft", Version: "1.0.0", License: "GPL-3.0"},
		{Kind: "go", Name: "unrecognised", Version: "v2", License: ""},
		{Kind: "go", Name: "legacy-lib", Version: "v3", License: "GPL-3.0"},
		{Kind: "web", Name: "changed-lib", Version: "2.0.0", License: "AGPL-3.0"},
	}), "\n")
	for _, want := range []string{
		"web copyleft 1.0.0: license GPL-3.0 not allowed",
		"go unrecognised v2: license unknown not allowed",
		"web changed-lib 2.0.0: license AGPL-3.0, the exception of 2026-10-28 names MIT",
		"exception for gone-lib: the package is no longer shipped",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "fine") || strings.Contains(got, "legacy-lib") {
		t.Errorf("allowed or excepted dependency reported:\n%s", got)
	}
}

func TestGoLicense(t *testing.T) {
	agpl, err := os.ReadFile("../../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"MIT", map[string]string{"LICENSE": mitText}, "MIT"},
		{"AGPL", map[string]string{"COPYING": string(agpl)}, "AGPL-3.0"},
		{"two files", map[string]string{"LICENSE-MIT": mitText, "LICENSE-AGPL.txt": string(agpl), "license.go": "package x"}, "AGPL-3.0 AND MIT"},
		{"own words", map[string]string{"LICENSE": "All rights reserved. Ask us before you use this."}, ""},
		{"no file", map[string]string{"README.md": mitText}, ""},
		{"one file unrecognised", map[string]string{"LICENSE": mitText, "COPYING": "Ask us first."}, ""},
	} {
		dir := t.TempDir()
		for f, text := range c.files {
			if err := os.WriteFile(filepath.Join(dir, f), []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := goLicense(dir); err != nil || got != c.want {
			t.Errorf("%s: %q %v, want %q", c.name, got, err, c.want)
		}
	}
}

// The acceptance of PL29: a shipped dependency with a license that is not allowed makes the
// step fail and names it; the dependencies of today pass.
func TestRun(t *testing.T) {
	if testing.Short() {
		t.Skip("lists the Go dependencies with go list")
	}
	web := filepath.Join(t.TempDir(), "web.json")
	write := func(json string) {
		t.Helper()
		if err := os.WriteFile(web, []byte(json), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The fonts of UI29 are exceptions in licenses.txt; an exception without its package fails.
	write(`{"MIT": [{"name": "react", "versions": ["19.3.1"], "license": "MIT"}],
	         "(MIT OR CC0-1.0)": [{"name": "dual", "versions": ["1.0.0"], "license": "(MIT OR CC0-1.0)"}],
	         "OFL-1.1": [{"name": "@fontsource/ibm-plex-sans", "versions": ["5.3.0"], "license": "OFL-1.1"},
	                     {"name": "@fontsource/ibm-plex-mono", "versions": ["5.3.0"], "license": "OFL-1.1"}]}`)
	var out bytes.Buffer
	if code := run(t.Context(), []string{"--go=group24.de/itsm/cmd/itsm", "--web=" + web}, &out); code != 0 {
		t.Fatalf("today's dependencies: %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "licenses ok") || !strings.Contains(out.String(), "4 web packages") {
		t.Fatalf("output:\n%s", out.String())
	}

	write(`{"MIT": [{"name": "react", "versions": ["19.3.1"], "license": "MIT"}],
	         "GPL-3.0": [{"name": "copyleft-widget", "versions": ["2.1.0"], "license": "GPL-3.0"}],
	         "Unknown": [{"name": "mystery", "versions": ["0.1.0"], "license": "Unknown"}]}`)
	out.Reset()
	if code := run(t.Context(), []string{"--go=group24.de/itsm/cmd/itsm", "--web=" + web}, &out); code != 1 {
		t.Fatalf("GPL dependency: %d\n%s", code, out.String())
	}
	for _, want := range []string{"web copyleft-widget 2.1.0: license GPL-3.0 not allowed", "web mystery 0.1.0: license unknown not allowed", "licenses not ok"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in\n%s", want, out.String())
		}
	}
}
