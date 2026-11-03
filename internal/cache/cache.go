// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package cache connects to Valkey (Hauptplan E20, E56). Later it adds the in-process level
// and invalidation over Pub/Sub (architektur.md §2, "Cache").
package cache

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/valkey-io/valkey-go"
)

// Client wraps the Valkey client so that the rest of the code does not depend on it directly.
type Client struct {
	c valkey.Client
	// channels is put before every Pub/Sub channel: "db<n>:" for a selected database n, because
	// Valkey keeps keys per database but channels for the whole server (PL52).
	channels string
}

// Open connects to Valkey and checks that it answers. "host:port/n" selects the database n,
// so that parts of the CI share one server without sharing keys (PL52).
func Open(ctx context.Context, addr string) (*Client, error) {
	opt := valkey.ClientOption{InitAddress: []string{addr}}
	if host, db, ok := strings.Cut(addr, "/"); ok {
		n, err := strconv.Atoi(db)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("cache: database %q of %s is no number", db, addr)
		}
		opt.InitAddress, opt.SelectDB = []string{host}, n
	}
	c, err := valkey.NewClient(opt)
	if err != nil {
		return nil, fmt.Errorf("cache: connect %s: %w", addr, err)
	}
	cl := &Client{c: c}
	if opt.SelectDB > 0 {
		cl.channels = fmt.Sprintf("db%d:", opt.SelectDB)
	}
	if err := cl.Ping(ctx); err != nil {
		c.Close()
		return nil, err
	}
	return cl, nil
}

// Ping checks the connection.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.c.Do(ctx, c.c.B().Ping().Build()).Error(); err != nil {
		return fmt.Errorf("cache: ping: %w", err)
	}
	return nil
}

// Close releases all connections.
func (c *Client) Close() { c.c.Close() }
