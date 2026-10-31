// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package db

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

type tenantKey struct{}

// The router of PL47: the tenant of the request picks its database, a node below the tenant
// counts as the tenant, the root and unknown tenants go to the control database; one database
// answers everything with itself.
func TestRouter(t *testing.T) {
	ctx := t.Context()
	pool := func() *pgxpool.Pool {
		// No connection is made before the first query.
		p, err := pgxpool.New(ctx, "postgres://itsm@127.0.0.1:1/none")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	control, a, b := pool(), pool(), pool()
	r := NewRouter(control, map[string]*pgxpool.Pool{"root.a": a, "root.b": b},
		func(ctx context.Context) string { s, _ := ctx.Value(tenantKey{}).(string); return s })
	in := func(tenant string) context.Context { return context.WithValue(ctx, tenantKey{}, tenant) }
	for _, c := range []struct {
		tenant string
		want   *pgxpool.Pool
	}{
		{"root.a", a}, {"root.a.x.y", a}, {"root.b", b}, {"root", control}, {"", control}, {"root.c", control},
	} {
		if got := r.Tenant(in(c.tenant)); got != c.want {
			t.Errorf("tenant %q: wrong database", c.tenant)
		}
		if got := r.For(c.tenant); got != c.want {
			t.Errorf("for %q: wrong database", c.tenant)
		}
	}
	if r.Control() != control || !r.Separate() {
		t.Fatal("control")
	}
	if ps := r.Pools(); len(ps) != 3 || ps[0] != control || ps[1] != a || ps[2] != b {
		t.Fatalf("pools: %d", len(ps))
	}
	// Background work bound to a database (PL47c) gets that one, whatever the session says.
	dbs := r.Databases()
	if len(dbs) != 3 || dbs[0].Name != "" || dbs[1].Name != "root.a" || dbs[2].Name != "root.b" || dbs[2].Pool() != b {
		t.Fatalf("databases: %+v", dbs)
	}
	if got := r.Tenant(dbs[2].Bind(in("root.a"))); got != b {
		t.Error("a bound context must get its database")
	}
	if cur := r.Current(in("root.a.x")); cur.Name != "root.a" || cur.Pool() != a {
		t.Errorf("current of root.a.x: %q", cur.Name)
	}
	if cur := r.Current(dbs[0].Bind(in("root.b"))); cur.Name != "" || cur.Pool() != control {
		t.Errorf("current bound to control: %q", cur.Name)
	}
	one := Single(control)
	if one.Tenant(in("root.a")) != control || one.For("root.b") != control || one.Separate() || len(one.Pools()) != 1 {
		t.Fatal("one database is control and tenant at once")
	}
}

// No service holds a pool or a query handle of its own (PL47): it holds a Router and picks the
// database at each access, so that the operating mode is one switch. Functions may take a
// pool as an argument (migrations, the schema of one database).
func TestNoServiceHoldsAPool(t *testing.T) {
	root := filepath.Join("..", "..", "internal")
	fset := token.NewFileSet()
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Dir(path) == filepath.Join(root, "db") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				if ty := types.ExprString(field.Type); ty == "*pgxpool.Pool" || ty == "*queries.Queries" {
					found = append(found, fset.Position(field.Pos()).String()+" "+ty)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Fatalf("services hold a database of their own; use *db.Router:\n%s", strings.Join(found, "\n"))
	}
}
