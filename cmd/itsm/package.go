// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/customer"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/platform"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/schema"
)

// packageCmd handles "itsm package install [--force] <dir>", "itsm package list" and
// "itsm package export-customer <dir>".
func packageCmd(args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("package needs a subcommand: install [--force] <dir> | list | export-customer <dir>")
	}
	cfg, err := platform.LoadConfig(getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rt, closeDBs, err := openDatabases(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeDBs()

	switch args[0] {
	case "install":
		rest, force := args[1:], false
		if len(rest) == 2 && rest[0] == "--force" {
			rest, force = rest[1:], true
		}
		if len(rest) != 1 {
			return errors.New("usage: itsm package install [--force] <dir>")
		}
		// Logs go to stderr so that stdout stays readable for the operator.
		return packageInstall(ctx, rt, rest[0], force, platform.NewLogger(stderr, cfg.LogLevel), stdout)
	case "export-customer":
		if len(args) != 2 {
			return errors.New("usage: itsm package export-customer <dir>")
		}
		return exportCustomer(ctx, rt, args[1], platform.NewLogger(stderr, cfg.LogLevel), stdout)
	case "list":
		return packageList(ctx, queries.New(rt.Control()), stdout)
	default:
		return fmt.Errorf("unknown package subcommand %q", args[0])
	}
}

// errCustomerConflicts stops an install that would break changes of the customer layer.
var errCustomerConflicts = errors.New("the package conflicts with the customer layer; install with --force to still these changes")

func packageInstall(ctx context.Context, rt *db.Router, dir string, force bool, logger *slog.Logger, stdout io.Writer) error {
	p, err := meta.Load(dir)
	if err != nil {
		return fmt.Errorf("package %s: %w", dir, err)
	}
	pool := rt.Control()
	before, err := meta.Types(ctx, queries.New(pool))
	if err != nil {
		return err
	}
	// The customer layer before the update (PL43, E37): changes that would no longer apply
	// stop the install, or with --force are stilled, visible but without effect.
	conflicts, err := customer.UpdateConflicts(ctx, queries.New(pool), p)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		fmt.Fprintf(stdout, "Konflikte mit der Kundenschicht:\n")
		names := make([]string, 0, len(conflicts))
		for name := range conflicts {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			for _, c := range conflicts[name] {
				fmt.Fprintf(stdout, "  %s: %s %s (%s): %s\n", name, c.Change.Op, c.Change.Field, c.Change.ID, c.Reason)
			}
		}
		if !force {
			return errCustomerConflicts
		}
	}
	if err := meta.Store(ctx, pool, p); err != nil {
		return err
	}
	if len(conflicts) > 0 {
		if err := customer.Still(ctx, queries.New(pool), conflicts); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Die Änderungen oben sind stillgelegt.\n")
	}
	// The tables of the types go into every database (PL47d); the definitions stay central.
	var changes []schema.Change
	for _, d := range rt.Pools() {
		c, err := schema.Apply(ctx, d, p.Types, logger)
		if err != nil {
			return err
		}
		changes = append(changes, c...)
	}
	seeded, err := records.New(rt).SeedLists(ctx, p)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Paket %s %s installiert, %d Schemaänderungen, %d Listeneinträge angelegt\n", p.Name, p.Version, len(changes), seeded)
	if err := recomputeChanged(ctx, rt, before, p, logger, stdout); err != nil {
		return err
	}
	for _, t := range p.Types {
		kind := "Typ"
		if t.IsTree() {
			kind = "Auswahlliste (Baum)"
		} else if t.IsList() {
			kind = "Auswahlliste"
		}
		traits := ""
		if len(t.Traits) > 0 {
			traits = ", Merkmale " + strings.Join(t.Traits, ", ")
		}
		fmt.Fprintf(stdout, "  %s %s (%s) mit %d Feldern%s:\n", kind, t.Name, t.Label["de"], len(t.Fields), traits)
		for _, f := range t.Fields {
			var extra []string
			if f.Required {
				extra = append(extra, "Pflicht")
			}
			if f.Unique {
				extra = append(extra, "eindeutig")
			}
			if f.Type == meta.FieldReference {
				extra = append(extra, "→ "+f.To)
			}
			if f.Type == meta.FieldChoice {
				extra = append(extra, fmt.Sprintf("%d Optionen", len(f.Options)))
			}
			if f.Trait != "" {
				extra = append(extra, "aus "+f.Trait)
			}
			if f.Formula != "" {
				extra = append(extra, "berechnet")
			}
			suffix := ""
			if len(extra) > 0 {
				suffix = " [" + strings.Join(extra, ", ") + "]"
			}
			fmt.Fprintf(stdout, "    %-16s %-10s %s%s\n", f.Name, f.Type, f.Label["de"], suffix)
		}
	}
	return nil
}

// recomputeChanged queues the job records.recompute for every type whose formulas are new or
// changed against the definitions installed before (PL22, E99), and with Search for every type
// whose searched fields changed or that has records without a search column (PL26, E101). A
// type that did not exist has no records and needs none.
func recomputeChanged(ctx context.Context, rt *db.Router, before []meta.Type, p meta.Package, logger *slog.Logger, stdout io.Writer) error {
	js := jobs.New(rt, jobs.Options{Logger: logger})
	if err := js.Start(ctx); err != nil {
		return err
	}
	// Each database recomputes its own records (PL47d).
	for _, d := range rt.Databases() {
		if err := recomputeIn(d.Bind(ctx), rt, js, before, p, stdout); err != nil {
			return err
		}
	}
	return nil
}

// recomputeIn queues the recomputation of the changed types in the database of the context.
func recomputeIn(ctx context.Context, rt *db.Router, js *jobs.Service, before []meta.Type, p meta.Package, stdout io.Writer) error {
	old := map[string]meta.Type{}
	for _, t := range before {
		old[t.QualifiedName()] = t
	}
	rec := records.New(rt)
	var due []records.RecomputeArgs
	for _, t := range p.Types {
		prev, existed := old[t.QualifiedName()]
		if !existed {
			continue
		}
		args := records.RecomputeArgs{Type: t.QualifiedName()}
		formulas := false
		for _, f := range t.Fields {
			if pf, _ := prev.Field(f.Name); f.Formula != "" && f.Formula != pf.Formula {
				formulas = true
				break
			}
		}
		if searchSignature(t) != searchSignature(prev) {
			args.Search = true
		} else if unfilled, err := rec.SearchUnfilled(ctx, t.QualifiedName()); err != nil {
			return err
		} else {
			args.Search = unfilled
		}
		if formulas || args.Search {
			due = append(due, args)
		}
	}
	if len(due) == 0 {
		return nil
	}
	for _, args := range due {
		job, err := js.Enqueue(ctx, args, &river.InsertOpts{Queue: jobs.QueueBulk})
		if err != nil {
			return err
		}
		what := "Berechnete Felder"
		if args.Search {
			what = "Berechnete Felder und Suchspalte"
		}
		fmt.Fprintf(stdout, "  %s von %s werden neu berechnet (Job %d)\n", what, args.Type, job.ID)
	}
	return nil
}

// searchSignature names what makes up the search column of a type.
func searchSignature(t meta.Type) string {
	title, others := t.SearchFields()
	sig := title + "|" + strings.Join(others, ",")
	for _, f := range t.Fields {
		if f.Translatable {
			sig += "|t:" + f.Name
		}
	}
	return sig
}

func packageList(ctx context.Context, q *queries.Queries, stdout io.Writer) error {
	pkgs, err := q.ListPackages(ctx)
	if err != nil {
		return err
	}
	types, err := meta.Types(ctx, q)
	if err != nil {
		return err
	}
	count := map[string]int{}
	for _, t := range types {
		count[t.Package]++
	}
	if len(pkgs) == 0 {
		fmt.Fprintln(stdout, "Keine Pakete installiert")
		return nil
	}
	for _, p := range pkgs {
		fmt.Fprintf(stdout, "%-20s %-10s %d Typen, installiert %s\n", p.Name, p.Version, count[p.Name],
			p.InstalledAt.Time.Format("2006-01-02 15:04"))
	}
	return nil
}

// exportCustomer writes the customer layers as a directory: package.yaml naming the packages
// the changes extend, and customer/<package>.<type>.yaml per type with its changes, in order.
// Installing it comes with EX (architektur.md §4).
func exportCustomer(ctx context.Context, rt *db.Router, dir string, logger *slog.Logger, stdout io.Writer) error {
	views, err := customer.New(rt, records.New(rt), logger).Export(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "customer"), 0o750); err != nil { //nolint:gosec // G703: the operator names the directory
		return err
	}
	extends := map[string]bool{}
	for _, v := range views {
		pkg, _, _ := strings.Cut(v.Type, ".")
		extends[pkg] = true
		list := make([]meta.Change, 0, len(v.Changes))
		for _, e := range v.Changes {
			list = append(list, e.Change)
		}
		// Through JSON, so that the YAML carries the names of the API.
		raw, err := json.Marshal(list)
		if err != nil {
			return err
		}
		var changes []any
		if err := json.Unmarshal(raw, &changes); err != nil {
			return err
		}
		b, err := yaml.Marshal(map[string]any{"type": v.Type, "version": v.Version, "changes": changes})
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "customer", v.Type+".yaml"), b, 0o600); err != nil { //nolint:gosec // G703: the operator names the directory
			return err
		}
	}
	names := make([]string, 0, len(extends))
	for n := range extends {
		names = append(names, n)
	}
	sort.Strings(names)
	manifest, err := yaml.Marshal(map[string]any{"name": "customer", "version": time.Now().UTC().Format("2006.1.2"),
		"label": map[string]string{"de": "Kundenschicht", "en": "Customer layer"}, "layer": "customer", "extends": names})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "package.yaml"), manifest, 0o600); err != nil { //nolint:gosec // G703: the operator names the directory
		return err
	}
	fmt.Fprintf(stdout, "Kundenschicht exportiert: %d Typen nach %s\n", len(views), dir)
	return nil
}
