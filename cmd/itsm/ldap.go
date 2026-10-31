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
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/platform"
	"group24.de/itsm/internal/records"
)

// itsm ldap list | ldap sync [name]: the directories and a sync in the foreground (PL12).
func ldapCmd(args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("ldap needs a subcommand: list | sync [name]")
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
	logger := platform.NewLogger(stderr, cfg.LogLevel)
	az := authz.New(rt, nil, logger)
	an := authn.New(rt, nil, auth.New(rt, nil, logger), az, records.New(rt), org.New(rt), logger, authn.Options{})

	switch args[0] {
	case "list":
		list, err := an.Directories(ctx)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Fprintln(stdout, "keine Verzeichnisse")
			return nil
		}
		for _, d := range list {
			state := "aktiv"
			if !d.Enabled {
				state = "aus"
			}
			last := "noch nie"
			if d.LastSyncAt != nil {
				last = d.LastSyncAt.Format("2006-01-02 15:04:05")
			}
			fmt.Fprintf(stdout, "%-20s %-8s %s  %s  letzter Abgleich: %s\n", d.Name, state, d.URL, d.BaseDN, last)
		}
		return nil
	case "sync":
		if len(args) > 2 {
			return errors.New("usage: itsm ldap sync [name]")
		}
		if name := strings.Join(args[1:], ""); name != "" {
			res, err := an.Sync(ctx, name)
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout, res.String())
			return nil
		}
		results, err := an.SyncAll(ctx)
		for _, r := range results {
			fmt.Fprintln(stdout, r.String())
		}
		return err
	default:
		return fmt.Errorf("unknown ldap subcommand %q", args[0])
	}
}
