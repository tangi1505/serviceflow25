// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package dbtest gives a test a database of its own. The test packages share one database and
// run at the same time; a test that changes what all of them see (the retention deleting the
// start of the event log) takes a fresh one on the same server instead.
package dbtest

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Isolated creates the database name next to the one of the URL, empty, and returns its URL;
// it is dropped when the test ends. A database left over from a run that was killed is
// dropped first.
func Isolated(t testing.TB, base, name string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	ident := pgx.Identifier{name}.Sanitize()
	for _, sql := range []string{"DROP DATABASE IF EXISTS " + ident + " WITH (FORCE)", "CREATE DATABASE " + ident} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, base)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		// FORCE ends the connections the test's pools may still hold.
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	return u.String()
}
