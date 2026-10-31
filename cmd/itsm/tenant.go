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

	"github.com/jackc/pgx/v5/pgxpool"

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
//	itsm tenant check <name>       with a database per tenant: does it fit the control database?
//
// With ITSM_TENANT_MODE=database a new tenant gets the empty database the operator made for it
// at ITSM_TENANT_<NAME>_DATABASE_URL (PL47d, Hauptplan E144); restart the instances afterwards,
// they open the tenants' databases at start. The access of running instances follows within
// seconds (authz cache, entryTTL).
func tenantCmd(args []string, getenv func(string) string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("tenant needs a subcommand: create <name> [--label …] [--adopt [--admin <user>]…] | list | check <name>")
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
	sec := seclog.New(db.Single(pool), nil, platform.NewLogger(io.Discard, cfg.LogLevel))
	if cfg.SeparateDatabases() && args[0] == "check" {
		return tenantCheck(ctx, cfg, pool, args[1:], stdout)
	}

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
		o := tenant.Options{Name: args[1], Label: label, Adopt: *adopt, Admins: admins}
		var res tenant.Result
		if cfg.SeparateDatabases() {
			// The tenant gets the database the operator made for it (PL47d, E144).
			res, err = tenant.CreateIn(ctx, pool, cfg.TenantURL(args[1]), o, platform.NewLogger(io.Discard, cfg.LogLevel))
		} else {
			res, err = tenant.Create(ctx, pool, o)
		}
		if err != nil {
			if errors.Is(err, db.ErrNoDatabase) {
				return fmt.Errorf("%w; set %s to the empty database made for it", err, platform.TenantDatabaseVar(o.Name))
			}
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
		list, err := org.New(db.Single(pool)).Tenants(ctx)
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
	case "check":
		return errors.New("itsm tenant check compares a tenant's own database with the control database; it needs ITSM_TENANT_MODE=database")
	default:
		return fmt.Errorf("unknown tenant subcommand %q", args[0])
	}
}

// tenantCheck compares the database of a tenant with the control database, after restoring it
// from a backup (PL47d): it prints what does not fit and fails when anything does.
func tenantCheck(ctx context.Context, cfg platform.Config, control *pgxpool.Pool, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: itsm tenant check <name>")
	}
	path := tenant.Path(args[0])
	name := strings.TrimPrefix(path, org.Root+".")
	url := cfg.TenantURL(name)
	if url == "" {
		return fmt.Errorf("%w: %s; set %s", db.ErrNoDatabase, path, platform.TenantDatabaseVar(name))
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	findings, err := tenant.Check(ctx, control, pool, path)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		fmt.Fprintf(stdout, "Mandant %s: Datenbank passt zur Steuer-Datenbank\n", path)
		return nil
	}
	for _, f := range findings {
		fmt.Fprintf(stdout, "%-10s %s\n", f.What, f.Detail)
	}
	return fmt.Errorf("tenant %s: %d findings", path, len(findings))
}
