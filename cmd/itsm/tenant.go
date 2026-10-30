// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/platform"
	"group24.de/itsm/internal/seclog"
	"group24.de/itsm/internal/tenant"
)

// adminList collects repeated --admin flags.
type adminList []string

func (l *adminList) String() string     { return strings.Join(*l, ",") }
func (l *adminList) Set(v string) error { *l = append(*l, v); return nil }

// tenantCmd manages the tenants of the installation (PL44, Hauptplan E136):
//
//	itsm tenant create <name> --label "Polizei Nord" [--label-en "Police North"]
//	itsm tenant create <name> --label … --adopt [--admin pascal]…   the first one takes over all
//	itsm tenant list
//
// The access of running instances follows within seconds (authz cache, entryTTL).
func tenantCmd(args []string, getenv func(string) string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("tenant needs a subcommand: create <name> [--label …] [--adopt [--admin <user>]…] | list")
	}
	cfg, err := platform.LoadConfig(getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	sec := seclog.New(pool, nil, platform.NewLogger(io.Discard, cfg.LogLevel))

	switch args[0] {
	case "create":
		if len(args) < 2 || strings.HasPrefix(args[1], "-") {
			return errors.New("usage: itsm tenant create <name> [--label …] [--label-en …] [--adopt [--admin <user>]…]")
		}
		fs := flag.NewFlagSet("tenant create", flag.ContinueOnError)
		de := fs.String("label", "", "German name of the tenant")
		en := fs.String("label-en", "", "English name (default: the German one)")
		adopt := fs.Bool("adopt", false, "the first tenant takes over the tree, the records, the roles and every account")
		var admins adminList
		fs.Var(&admins, "admin", "with --adopt: this account stays an account of the installation (repeatable)")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		label := map[string]string{}
		if *de != "" {
			label["de"], label["en"] = *de, *de
		}
		if *en != "" {
			label["en"] = *en
		}
		res, err := tenant.Create(ctx, pool, tenant.Options{Name: args[1], Label: label, Adopt: *adopt, Admins: admins})
		if err != nil {
			return err
		}
		cliChanged(ctx, sec, "tenant.create", res.Tenant.Path, map[string]any{"after": map[string]any{
			"label": label, "adopt": *adopt, "admins": []string(admins), "nodes": res.Nodes, "assignments": res.Assignments, "accounts": res.Accounts}})
		fmt.Fprintf(stdout, "Mandant %s angelegt\n", res.Tenant.Path)
		if res.Seeded > 0 {
			fmt.Fprintf(stdout, "Auswahllisten der Pakete kopiert: %d Einträge\n", res.Seeded)
		}
		if *adopt {
			var moved int64
			names := make([]string, 0, len(res.Records))
			for name, n := range res.Records {
				moved += n
				if n > 0 {
					names = append(names, fmt.Sprintf("%s %d", name, n))
				}
			}
			sort.Strings(names)
			fmt.Fprintf(stdout, "Übernommen: %d Knoten, %d Datensätze, %d Zuweisungen, %d Konten\n", res.Nodes, moved, res.Assignments, res.Accounts)
			if len(names) > 0 {
				fmt.Fprintf(stdout, "  %s\n", strings.Join(names, ", "))
			}
			if len(admins) > 0 {
				fmt.Fprintf(stdout, "Konten der Installation: %s\n", strings.Join(admins, ", "))
			}
		}
		return nil
	case "list":
		list, err := org.New(pool).Tenants(ctx)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Fprintln(stdout, "keine Mandanten: die Wurzel ist der einzige")
		}
		for _, t := range list {
			fmt.Fprintf(stdout, "%-24s %s\n", t.Path, t.Label["de"])
		}
		return nil
	default:
		return fmt.Errorf("unknown tenant subcommand %q", args[0])
	}
}
