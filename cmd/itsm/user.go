// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/platform"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seclog"
	"group24.de/itsm/internal/tenant"
)

// cliChanged records a change made on the command line in the security event log (PL27):
// no account and no address, the user agent names the command line.
func cliChanged(ctx context.Context, sec *seclog.Log, kind, target string, details map[string]any) {
	sec.Record(ctx, seclog.Event{Category: seclog.Change, Kind: kind, Outcome: seclog.Success, Target: target,
		UserAgent: "itsm cli", Details: details})
}

// userCmd manages local accounts (PL5). The password is read from stdin, never from an
// argument, so that it does not show in the process list or the shell history:
//
//	printf 'Secret-1!' | itsm user create pascal --display-name "Pascal Frechen"
//	printf 'Secret-2!' | itsm user passwd pascal --expires 2026-12-31
//	itsm user disable pascal | itsm user enable pascal | itsm user list
//	itsm user grant pascal admin | itsm user link pascal <person-id>
//	itsm user tenant pascal nord | itsm user tenant pascal -   (bind to a tenant, PL44; - frees)
//	printf 'Secret-1!' | itsm user create anna --tenant nord
func userCmd(args []string, getenv func(string) string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("user needs a subcommand: create | passwd | disable | enable | link | grant | tenant | list")
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
	// Account commands do not touch sessions, so no Valkey connection is needed here.
	svc := auth.New(rt, nil, platform.NewLogger(io.Discard, cfg.LogLevel))
	sec := seclog.New(rt, nil, platform.NewLogger(io.Discard, cfg.LogLevel))

	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("user create", flag.ContinueOnError)
		display := fs.String("display-name", "", "name shown in the interface (default: the username)")
		expires := fs.String("expires", "", "date (YYYY-MM-DD) after which the password must be changed")
		home := fs.String("tenant", "", "tenant the account belongs to (PL44); without, an account of the installation")
		username, err := parseUser(fs, args[1:])
		if err != nil {
			return err
		}
		exp, err := parseExpiry(*expires)
		if err != nil {
			return err
		}
		pw, err := readPassword(stdin)
		if err != nil {
			return err
		}
		var acc auth.Account
		if *home != "" && rt.Separate() {
			// With a database per tenant the account is made in its tenant's database (PL47d).
			if err := tenant.IsTenant(ctx, rt.Control(), tenant.Path(*home)); err != nil {
				return err
			}
			acc, err = svc.CreateAccountIn(ctx, tenant.Path(*home), username, *display, pw, exp)
		} else {
			acc, err = svc.CreateAccount(ctx, username, *display, pw, exp)
		}
		if err != nil {
			return err
		}
		if *home != "" && !rt.Separate() {
			if err := tenant.Bind(ctx, rt.Control(), username, *home); err != nil {
				return err
			}
		}
		cliChanged(ctx, sec, "account.create", acc.Username, map[string]any{"after": map[string]any{"id": acc.ID, "display_name": acc.DisplayName, "tenant": *home}})
		fmt.Fprintf(stdout, "Konto %s angelegt (%s)\n", acc.Username, acc.ID)
		return nil
	case "tenant":
		// itsm user tenant <username> <tenant|->
		if len(args) != 3 {
			return errors.New("usage: itsm user tenant <username> <tenant|->; - makes it an account of the installation")
		}
		to := args[2]
		if to == "-" {
			to = ""
		}
		if rt.Separate() {
			// An account lives in its tenant's database and cannot move (PL47b, PL47d).
			return errors.New("with a database per tenant an account keeps its tenant; create a new account in the other tenant")
		}
		if err := tenant.Bind(ctx, rt.Control(), args[1], to); err != nil {
			return err
		}
		cliChanged(ctx, sec, "account.tenant", args[1], map[string]any{"after": map[string]any{"tenant": to}})
		if to == "" {
			fmt.Fprintf(stdout, "Konto %s gehört zur Installation\n", args[1])
		} else {
			fmt.Fprintf(stdout, "Konto %s gehört zum Mandanten %s\n", args[1], tenant.Path(to))
		}
		return nil
	case "passwd":
		fs := flag.NewFlagSet("user passwd", flag.ContinueOnError)
		expires := fs.String("expires", "", "date (YYYY-MM-DD) after which the password must be changed; empty clears it")
		username, err := parseUser(fs, args[1:])
		if err != nil {
			return err
		}
		exp, err := parseExpiry(*expires)
		if err != nil {
			return err
		}
		pw, err := readPassword(stdin)
		if err != nil {
			return err
		}
		if err := svc.SetPassword(ctx, username, pw, exp); err != nil {
			return err
		}
		cliChanged(ctx, sec, "account.password", username, map[string]any{"after": map[string]any{"expires": exp}})
		fmt.Fprintf(stdout, "Kennwort von %s gesetzt\n", username)
		return nil
	case "disable", "enable":
		if len(args) != 2 {
			return fmt.Errorf("usage: itsm user %s <username>", args[0])
		}
		if err := svc.SetDisabled(ctx, args[1], args[0] == "disable"); err != nil {
			return err
		}
		cliChanged(ctx, sec, "account."+args[0], args[1], nil)
		fmt.Fprintf(stdout, "Konto %s: %s\n", args[1], map[string]string{"disable": "gesperrt", "enable": "freigegeben"}[args[0]])
		return nil
	case "link":
		// itsm user link <username> <person-id>
		if len(args) != 3 {
			return errors.New("usage: itsm user link <username> <person-id>")
		}
		if err := userLink(ctx, rt, args[1], args[2], stdout); err != nil {
			return err
		}
		cliChanged(ctx, sec, "account.link_person", args[1], map[string]any{"after": map[string]any{"person": args[2]}})
		return nil
	case "grant":
		// itsm user grant <username> <role> [--node root]: assigns the role to the account's
		// person at the node; an account without a person gets one (core.person) named after it.
		// Without --node the node is the account's tenant, else the root (PL44).
		fs := flag.NewFlagSet("user grant", flag.ContinueOnError)
		node := fs.String("node", "", "node of the organisation tree the role applies to (default: the account's tenant, else root)")
		if len(args) < 3 || strings.HasPrefix(args[1], "-") || strings.HasPrefix(args[2], "-") {
			return errors.New("usage: itsm user grant <username> <role> [--node root]")
		}
		if err := fs.Parse(args[3:]); err != nil {
			return err
		}
		if err := userGrant(ctx, rt, args[1], args[2], *node, stdout); err != nil {
			return err
		}
		cliChanged(ctx, sec, "assignment.create", args[2], map[string]any{"after": map[string]any{"username": args[1], "org_path": *node}})
		return nil
	case "list":
		accs, err := svc.ListAccounts(ctx)
		if err != nil {
			return err
		}
		for _, a := range accs {
			state := "aktiv"
			if a.DisabledAt != nil {
				state = "gesperrt"
			}
			exp := "-"
			if a.PasswordExpiresAt != nil {
				exp = a.PasswordExpiresAt.Format("2006-01-02")
			}
			home := a.Tenant
			if home == "" {
				home = "Installation"
			}
			fmt.Fprintf(stdout, "%-24s %-32s %-9s %-20s Kennwort läuft ab: %s\n", a.Username, a.DisplayName, state, home, exp)
		}
		return nil
	default:
		return fmt.Errorf("unknown user subcommand %q", args[0])
	}
}

func userLink(ctx context.Context, rt *db.Router, username, personID string, stdout io.Writer) error {
	pid, err := uuid.Parse(personID)
	if err != nil {
		return fmt.Errorf("person id: %w", err)
	}
	acc, err := accountByName(ctx, rt, username)
	if err != nil {
		return err
	}
	if err := authz.New(rt, nil, nil).LinkPerson(ctx, acc.ID, &pid); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Konto %s mit Person %s verknüpft\n", username, pid)
	return nil
}

func userGrant(ctx context.Context, rt *db.Router, username, role, node string, stdout io.Writer) error {
	acc, err := accountByName(ctx, rt, username)
	if err != nil {
		return err
	}
	az := authz.New(rt, nil, nil)
	// The account's tenant is where its person lives and, without --node, where the role goes.
	home := org.Root
	if acc.Tenant.Valid {
		home = acc.Tenant.String
	}
	if node == "" {
		node = home
	}
	var pid uuid.UUID
	if acc.PersonID.Valid {
		pid = acc.PersonID.UUID
	} else {
		// The person lives with the account, in its tenant's database (PL47d).
		rec, err := records.New(rt).Create(rt.BindFor(ctx, home), "core.person", map[string]any{"last_name": acc.DisplayName, "org_path": home}, nil)
		if err != nil {
			return fmt.Errorf("create person for %s (is packages/core installed?): %w", username, err)
		}
		pid = uuid.MustParse(rec["id"].(string))
		if err := az.LinkPerson(ctx, acc.ID, &pid); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Person %s für Konto %s angelegt\n", pid, username)
	}
	a, err := az.Assign(ctx, role, &pid, nil, node, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Rolle %s für %s an %s zugewiesen (%s)\n", role, username, a.OrgPath, a.ID)
	return nil
}

// accountByName reads an account in its home database (PL47b).
func accountByName(ctx context.Context, rt *db.Router, username string) (queries.Account, error) {
	home, err := rt.User(ctx, username)
	if err != nil {
		return queries.Account{}, fmt.Errorf("account %q: %w", username, err)
	}
	acc, err := queries.New(home).GetAccountByUsername(ctx, username)
	if err != nil {
		return queries.Account{}, fmt.Errorf("account %q: %w", username, err)
	}
	return acc, nil
}

func parseUser(fs *flag.FlagSet, args []string) (string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", fmt.Errorf("usage: itsm %s <username> [flags]; the password is read from stdin", fs.Name())
	}
	if err := fs.Parse(args[1:]); err != nil {
		return "", err
	}
	return args[0], nil
}

func parseExpiry(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // no expiry is a valid answer
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, fmt.Errorf("--expires: %w", err)
	}
	return &t, nil
}

// readPassword takes the first line of stdin, without the line ending.
func readPassword(stdin io.Reader) (string, error) {
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	pw := strings.TrimRight(line, "\r\n")
	if pw == "" {
		return "", errors.New("password expected on stdin, e.g. printf 'Secret-1!' | itsm user create <username>")
	}
	return pw, nil
}
