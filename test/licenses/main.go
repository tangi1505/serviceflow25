// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Command licenses checks the licenses of what itsm ships (PL29, Hauptplan E15): the Go
// modules linked into the binary of cmd/itsm and the web packages that end up in web/dist.
//
//	(cd web && pnpm licenses list --prod --json) >/tmp/web-licenses.json
//	go run ./test/licenses --web=/tmp/web-licenses.json [--report]
//
// The licenses of Go modules are recognised from their license files with
// github.com/google/licensecheck; the web packages bring theirs in package.json. Allowed
// licenses and the exceptions are in licenses.txt next to this file. Every finding names the
// package and its license, and the exit status is 1. Development tools are not checked: they
// are not shipped.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/licensecheck"
)

//go:embed licenses.txt
var policyText string

// minCoverage is how much of a license file must match known licenses, in percent. The rest
// is copyright lines and project notes; the files of the modules in use on 2026-10-28 were all
// above 80 %.
const minCoverage = 70

// dep is one shipped dependency with its license as an SPDX expression ("" = not recognised).
type dep struct {
	Kind, Name, Version, License string
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout))
}

func run(ctx context.Context, args []string, out io.Writer) int {
	fs := flag.NewFlagSet("licenses", flag.ContinueOnError)
	fs.SetOutput(out)
	pkgs := fs.String("go", "./cmd/itsm", "Go packages whose module dependencies are checked")
	web := fs.String("web", "", "output of `pnpm licenses list --prod --json`; empty = no web check")
	report := fs.Bool("report", false, "list every dependency with its license")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, err := parsePolicy(policyText)
	if err != nil {
		fmt.Fprintln(out, err)
		return 2
	}
	deps, err := goDeps(ctx, *pkgs)
	if err != nil {
		fmt.Fprintln(out, err)
		return 2
	}
	nGo := len(deps)
	if *web != "" {
		w, err := webDeps(*web)
		if err != nil {
			fmt.Fprintln(out, err)
			return 2
		}
		deps = append(deps, w...)
	}
	if *report {
		for _, d := range deps {
			fmt.Fprintf(out, "%-4s %-55s %-12s %s\n", d.Kind, d.Name, d.Version, orUnknown(d.License))
		}
	}
	counts := map[string]int{}
	for _, d := range deps {
		counts[orUnknown(d.License)]++
	}
	fmt.Fprintf(out, "licenses: %d Go modules, %d web packages\n", nGo, len(deps)-nGo)
	for _, l := range sortedKeys(counts) {
		fmt.Fprintf(out, "  %-30s %d\n", l, counts[l])
	}
	findings := p.check(deps)
	for _, f := range findings {
		fmt.Fprintln(out, f)
	}
	if len(findings) > 0 {
		fmt.Fprintln(out, "licenses not ok: fix the dependency or add an exception to test/licenses/licenses.txt (Hauptplan E15)")
		return 1
	}
	fmt.Fprintln(out, "licenses ok")
	return 0
}

func orUnknown(license string) string {
	if license == "" {
		return "unknown"
	}
	return license
}

// --- the policy ---

type exception struct{ License, Date, Reason string }

type policy struct {
	allowed    map[string]bool
	exceptions map[string]exception
}

func parsePolicy(text string) (policy, error) {
	p := policy{allowed: map[string]bool{}, exceptions: map[string]exception{}}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		word, rest, _ := strings.Cut(line, " ")
		switch word {
		case "allow":
			if strings.TrimSpace(rest) == "" || strings.Contains(strings.TrimSpace(rest), " ") {
				return policy{}, fmt.Errorf("licenses.txt line %d: allow takes one SPDX identifier", i+1)
			}
			p.allowed[strings.TrimSpace(rest)] = true
		case "except":
			parts := strings.Split(rest, "|")
			if len(parts) != 4 {
				return policy{}, fmt.Errorf("licenses.txt line %d: except <package> | <license> | <date> | <reason>", i+1)
			}
			for j := range parts {
				parts[j] = strings.TrimSpace(parts[j])
				if parts[j] == "" {
					return policy{}, fmt.Errorf("licenses.txt line %d: empty field in the exception", i+1)
				}
			}
			p.exceptions[parts[0]] = exception{License: parts[1], Date: parts[2], Reason: parts[3]}
		default:
			return policy{}, fmt.Errorf("licenses.txt line %d: unknown word %q", i+1, word)
		}
	}
	return p, nil
}

// check returns one finding per dependency that is neither allowed nor excepted, and one per
// exception that no longer fits.
func (p policy) check(deps []dep) []string {
	var findings []string
	used := map[string]bool{}
	for _, d := range deps {
		if e, ok := p.exceptions[d.Name]; ok {
			used[d.Name] = true
			if e.License != orUnknown(d.License) {
				findings = append(findings, fmt.Sprintf("%s %s %s: license %s, the exception of %s names %s",
					d.Kind, d.Name, d.Version, orUnknown(d.License), e.Date, e.License))
			}
			continue
		}
		if !p.allows(d.License) {
			findings = append(findings, fmt.Sprintf("%s %s %s: license %s not allowed", d.Kind, d.Name, d.Version, orUnknown(d.License)))
		}
	}
	for _, name := range sortedKeys(p.exceptions) {
		if !used[name] {
			findings = append(findings, fmt.Sprintf("exception for %s: the package is no longer shipped, remove the exception", name))
		}
	}
	return findings
}

// allows evaluates an SPDX expression: OR needs one allowed side, AND needs both, and
// "X WITH exception" counts as X. Anything it cannot read is not allowed.
func (p policy) allows(expr string) bool {
	tokens := strings.Fields(strings.NewReplacer("(", " ( ", ")", " ) ").Replace(expr))
	ok, rest, err := p.orExpr(tokens)
	return err == nil && len(rest) == 0 && ok
}

var errSyntax = errors.New("SPDX expression not readable")

func (p policy) orExpr(t []string) (bool, []string, error) {
	ok, t, err := p.andExpr(t)
	for err == nil && len(t) > 0 && strings.EqualFold(t[0], "OR") {
		var right bool
		right, t, err = p.andExpr(t[1:])
		ok = ok || right
	}
	return ok, t, err
}

func (p policy) andExpr(t []string) (bool, []string, error) {
	ok, t, err := p.atom(t)
	for err == nil && len(t) > 0 && strings.EqualFold(t[0], "AND") {
		var right bool
		right, t, err = p.atom(t[1:])
		ok = ok && right
	}
	return ok, t, err
}

func (p policy) atom(t []string) (bool, []string, error) {
	if len(t) == 0 {
		return false, nil, errSyntax
	}
	if t[0] == "(" {
		ok, rest, err := p.orExpr(t[1:])
		if err != nil || len(rest) == 0 || rest[0] != ")" {
			return false, nil, errSyntax
		}
		return ok, rest[1:], nil
	}
	if t[0] == ")" || strings.EqualFold(t[0], "OR") || strings.EqualFold(t[0], "AND") {
		return false, nil, errSyntax
	}
	id, rest := t[0], t[1:]
	if len(rest) >= 2 && strings.EqualFold(rest[0], "WITH") {
		rest = rest[2:]
	}
	return p.allowed[id], rest, nil
}

// --- Go modules ---

// goDeps lists the modules of the packages' dependencies, without the main module and the
// standard library, and recognises their licenses.
func goDeps(ctx context.Context, pkgs string) ([]dep, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-json=Module", pkgs) //nolint:gosec // G204: the packages of the flag, run by the developer or the CI
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list %s: %w", pkgs, err)
	}
	type module struct {
		Path, Version, Dir string
		Main               bool
		Replace            *struct{ Path, Version, Dir string }
	}
	seen := map[string]bool{}
	var deps []dep
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var pkg struct{ Module *module }
		if err := dec.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list: %w", err)
		}
		m := pkg.Module
		if m == nil || m.Main || seen[m.Path] {
			continue
		}
		seen[m.Path] = true
		dir, version := m.Dir, m.Version
		if m.Replace != nil {
			dir, version = m.Replace.Dir, m.Replace.Version
		}
		license, err := goLicense(dir)
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", m.Path, err)
		}
		deps = append(deps, dep{Kind: "go", Name: m.Path, Version: version, License: license})
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Name < deps[j].Name })
	return deps, nil
}

// goLicense recognises the license files in the root of a module (LICENSE, LICENCE, COPYING,
// with any suffix). Several licenses, in one file or in several, all apply ("A AND B"). ""
// means no file or a file that is not clearly a known license.
func goLicense(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	ids := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !isLicenseFile(e.Name()) {
			continue
		}
		text, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304: a file of the module cache
		if err != nil {
			return "", err
		}
		cov := licensecheck.Scan(text)
		if cov.Percent < minCoverage || len(cov.Match) == 0 {
			return "", nil
		}
		for _, m := range cov.Match {
			ids[m.ID] = true
		}
	}
	return strings.Join(sortedKeys(ids), " AND "), nil
}

func isLicenseFile(name string) bool {
	name = strings.ToLower(name)
	if strings.HasSuffix(name, ".go") {
		return false
	}
	return strings.HasPrefix(name, "license") || strings.HasPrefix(name, "licence") || strings.HasPrefix(name, "copying")
}

// --- web packages ---

// webDeps reads the output of `pnpm licenses list --prod --json`: licenses as keys, the
// packages under each.
func webDeps(path string) ([]dep, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the file of the flag
	if err != nil {
		return nil, err
	}
	var byLicense map[string][]struct {
		Name     string   `json:"name"`
		Versions []string `json:"versions"`
		License  string   `json:"license"`
	}
	if err := json.Unmarshal(raw, &byLicense); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var deps []dep
	for key, pkgs := range byLicense {
		for _, pkg := range pkgs {
			license := pkg.License
			if license == "" {
				license = key
			}
			if strings.EqualFold(license, "Unknown") {
				license = ""
			}
			deps = append(deps, dep{Kind: "web", Name: pkg.Name, Version: strings.Join(pkg.Versions, ","), License: license})
		}
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Name < deps[j].Name })
	return deps, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
