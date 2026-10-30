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
	"strings"
	"syscall"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/platform"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seclog"
)

// itsm client create <id> [--name …] [--ip-range CIDR]… | client list | client rotate <id>:
// API clients from the command line (PL13); the secret is printed this once.
func clientCmd(args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("client needs a subcommand: create <id> | list | rotate <id>")
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
	logger := platform.NewLogger(stderr, cfg.LogLevel)
	az := authz.New(pool, nil, logger)
	an := authn.New(pool, nil, auth.New(pool, nil, logger), az, records.New(pool), org.New(pool), logger, authn.Options{})
	sec := seclog.New(pool, nil, logger)

	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("client create", flag.ContinueOnError)
		name := fs.String("name", "", "display name (default: the id)")
		ranges := fs.String("ip-range", "", "allowed CIDRs, comma separated (default: any)")
		rate := fs.Int("rate-limit", 0, "requests per minute (default: setting api.rate_limit)")
		if len(args) < 2 || strings.HasPrefix(args[1], "-") {
			return errors.New("usage: itsm client create <id> [--name …] [--ip-range CIDR,…] [--rate-limit n]")
		}
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		in := authn.ClientInput{Name: *name}
		if *ranges != "" {
			list := strings.Split(*ranges, ",")
			in.IPRanges = &list
		}
		if *rate > 0 {
			v := int32(*rate) //nolint:gosec // a flag of the operator
			in.RateLimit = &v
		}
		c, secret, err := an.CreateClient(ctx, args[1], in, nil)
		if err != nil {
			return err
		}
		cliChanged(ctx, sec, "api_client.create", c.ClientID, map[string]any{"after": c})
		fmt.Fprintf(stdout, "Zugang %s angelegt (Konto %s, Person %v)\nGeheimnis (nur jetzt sichtbar): %s\n", c.ClientID, c.AccountID, c.PersonID, secret)
		fmt.Fprintf(stdout, "Rollen: itsm user grant %s <rolle>\n", c.ClientID)
		return nil
	case "list":
		list, err := an.Clients(ctx)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Fprintln(stdout, "keine Zugänge")
			return nil
		}
		for _, c := range list {
			state := "aktiv"
			if !c.Enabled {
				state = "aus"
			}
			last := "noch nie"
			if c.LastUsedAt != nil {
				last = c.LastUsedAt.Format("2006-01-02 15:04:05")
			}
			fmt.Fprintf(stdout, "%-20s %-6s %-30s IP %v  zuletzt: %s\n", c.ClientID, state, c.Name, c.IPRanges, last)
		}
		return nil
	case "rotate":
		if len(args) != 2 {
			return errors.New("usage: itsm client rotate <id>")
		}
		secret, err := an.RotateSecret(ctx, args[1], nil)
		if err != nil {
			return err
		}
		cliChanged(ctx, sec, "api_client.rotate", args[1], nil)
		fmt.Fprintf(stdout, "Neues Geheimnis für %s (nur jetzt sichtbar): %s\n", args[1], secret)
		return nil
	default:
		return fmt.Errorf("unknown client subcommand %q", args[0])
	}
}
