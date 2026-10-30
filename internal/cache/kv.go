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
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
)

// ErrMiss is returned when a key does not exist.
var ErrMiss = errors.New("cache: key not found")

// Set stores a value with a time to live; ttl must be positive, Valkey rounds to seconds.
func (c *Client) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	secs := max(int64(ttl/time.Second), 1)
	if err := c.c.Do(ctx, c.c.B().Set().Key(key).Value(string(value)).ExSeconds(secs).Build()).Error(); err != nil {
		return fmt.Errorf("cache: set %s: %w", key, err)
	}
	return nil
}

// Get returns the value of a key, or ErrMiss.
func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	v, err := c.c.Do(ctx, c.c.B().Get().Key(key).Build()).AsBytes()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, ErrMiss
		}
		return nil, fmt.Errorf("cache: get %s: %w", key, err)
	}
	return v, nil
}

// Del removes a key; a missing key is not an error.
func (c *Client) Del(ctx context.Context, key string) error {
	if err := c.c.Do(ctx, c.c.B().Del().Key(key).Build()).Error(); err != nil {
		return fmt.Errorf("cache: del %s: %w", key, err)
	}
	return nil
}

// TTL returns the remaining time to live of a key; ErrMiss when it does not exist, zero when it
// has no expiry.
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	secs, err := c.c.Do(ctx, c.c.B().Ttl().Key(key).Build()).AsInt64()
	if err != nil {
		return 0, fmt.Errorf("cache: ttl %s: %w", key, err)
	}
	switch secs {
	case -2:
		return 0, ErrMiss
	case -1:
		return 0, nil
	}
	return time.Duration(secs) * time.Second, nil
}
