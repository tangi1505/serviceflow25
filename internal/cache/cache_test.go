// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package cache

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenAndPing(t *testing.T) {
	addr := os.Getenv("ITSM_TEST_VALKEY_ADDR")
	if addr == "" {
		t.Skip("ITSM_TEST_VALKEY_ADDR not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := Open(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOpenFailsWithoutServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := Open(ctx, "127.0.0.1:1"); err == nil {
		t.Fatal("expected an error")
	}
}

// "host:port/n" opens database n of the server (PL52): a key set there is not in database 0;
// a database that is no number is refused.
func TestOpenSelectsADatabase(t *testing.T) {
	addr := os.Getenv("ITSM_TEST_VALKEY_ADDR")
	if addr == "" {
		t.Skip("ITSM_TEST_VALKEY_ADDR not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	host, _, _ := strings.Cut(addr, "/")
	if _, err := Open(ctx, host+"/x"); err == nil {
		t.Fatal("a database that is no number was taken")
	}
	nine, err := Open(ctx, host+"/9")
	if err != nil {
		t.Fatal(err)
	}
	defer nine.Close()
	zero, err := Open(ctx, host)
	if err != nil {
		t.Fatal(err)
	}
	defer zero.Close()
	if err := nine.Set(ctx, "cachetest:db", []byte("neun"), time.Minute); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = nine.Del(context.Background(), "cachetest:db") }()
	if v, err := nine.Get(ctx, "cachetest:db"); err != nil || string(v) != "neun" {
		t.Fatalf("database 9: %q %v", v, err)
	}
	if v, err := zero.Get(ctx, "cachetest:db"); !errors.Is(err, ErrMiss) {
		t.Fatalf("database 0 sees the key of database 9: %q %v", v, err)
	}
}
