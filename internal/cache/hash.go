// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package cache

import (
	"context"
	"fmt"
	"time"
)

// HSetExpire sets one field of a hash and the time to live of the whole hash, in one round
// trip (UI8: who is on a record).
func (c *Client) HSetExpire(ctx context.Context, key, field string, value []byte, ttl time.Duration) error {
	secs := max(int64(ttl/time.Second), 1)
	for _, r := range c.c.DoMulti(ctx,
		c.c.B().Hset().Key(key).FieldValue().FieldValue(field, string(value)).Build(),
		c.c.B().Expire().Key(key).Seconds(secs).Build(),
	) {
		if err := r.Error(); err != nil {
			return fmt.Errorf("cache: hset %s: %w", key, err)
		}
	}
	return nil
}

// HGetAll returns every field of a hash; a missing hash is empty.
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	m, err := c.c.Do(ctx, c.c.B().Hgetall().Key(key).Build()).AsStrMap()
	if err != nil {
		return nil, fmt.Errorf("cache: hgetall %s: %w", key, err)
	}
	return m, nil
}

// HDel removes fields of a hash; missing ones are no error.
func (c *Client) HDel(ctx context.Context, key string, fields ...string) error {
	if len(fields) == 0 {
		return nil
	}
	if err := c.c.Do(ctx, c.c.B().Hdel().Key(key).Field(fields...).Build()).Error(); err != nil {
		return fmt.Errorf("cache: hdel %s: %w", key, err)
	}
	return nil
}
