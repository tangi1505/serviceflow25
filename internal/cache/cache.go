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

	"github.com/valkey-io/valkey-go"
)

// Client wraps the Valkey client so that the rest of the code does not depend on it directly.
type Client struct {
	c valkey.Client
}

// Open connects to Valkey and checks that it answers.
func Open(ctx context.Context, addr string) (*Client, error) {
	c, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}})
	if err != nil {
		return nil, fmt.Errorf("cache: connect %s: %w", addr, err)
	}
	cl := &Client{c: c}
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
