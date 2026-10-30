// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package cache

import (
	"context"
	"os"
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
