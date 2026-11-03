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

// Incr adds one to a counter key and returns the new value (0 becomes 1 on a missing key).
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	n, err := c.c.Do(ctx, c.c.B().Incr().Key(key).Build()).AsInt64()
	if err != nil {
		return 0, fmt.Errorf("cache: incr %s: %w", key, err)
	}
	return n, nil
}

// IncrExpire adds one to a counter and gives a new key the ttl (rate limits, PL13).
func (c *Client) IncrExpire(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := c.Incr(ctx, key)
	if err != nil {
		return 0, err
	}
	if n == 1 {
		if err := c.c.Do(ctx, c.c.B().Expire().Key(key).Seconds(int64(ttl.Seconds())).Build()).Error(); err != nil {
			return n, fmt.Errorf("cache: expire %s: %w", key, err)
		}
	}
	return n, nil
}

// GetInt reads a counter; 0 when the key is missing.
func (c *Client) GetInt(ctx context.Context, key string) (int64, error) {
	b, err := c.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrMiss) {
			return 0, nil
		}
		return 0, err
	}
	var n int64
	if _, err := fmt.Sscanf(string(b), "%d", &n); err != nil {
		return 0, fmt.Errorf("cache: %s is not a number: %w", key, err)
	}
	return n, nil
}

// Publish sends a message on a channel to every subscriber (Hauptplan E20: Pub/Sub for
// invalidation across api instances).
func (c *Client) Publish(ctx context.Context, channel, message string) error {
	if err := c.c.Do(ctx, c.c.B().Publish().Channel(c.channels+channel).Message(message).Build()).Error(); err != nil {
		return fmt.Errorf("cache: publish %s: %w", channel, err)
	}
	return nil
}

// Subscribe calls fn for every message on the channel until the context ends. It blocks; run
// it in its own goroutine. The connection is a dedicated one, so other calls go on.
func (c *Client) Subscribe(ctx context.Context, channel string, fn func(message string)) error {
	err := c.c.Receive(ctx, c.c.B().Subscribe().Channel(c.channels+channel).Build(), func(msg valkey.PubSubMessage) {
		fn(msg.Message)
	})
	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("cache: subscribe %s: %w", channel, err)
	}
	return nil
}
