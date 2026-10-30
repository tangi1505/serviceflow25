// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package auth

import (
	"context"
	"errors"
	"net/netip"
	"strings"

	"group24.de/itsm/internal/cache"
)

// --- lockout after failed sign-ins (PL28, E103) ---
//
// Failed sign-ins count per username and per address in Valkey, each counter living one
// window. When a counter reaches its limit, a lock key holds the username or the address for
// the lockout time; a locked sign-in fails with the reason "locked" before the password is
// looked at. Unknown usernames count too, so that a lock tells nothing about which accounts
// exist. A success clears the counter of the username. Without Valkey there is no lockout.

const (
	failPrefix = "auth:fail:"
	lockPrefix = "auth:lock:"
)

func accountKey(username string) string { return "acct:" + strings.ToLower(username) }

func ipKey(ip netip.Addr) string { return "ip:" + ip.String() }

// locked reports whether the username or the address is locked.
func (s *Service) locked(ctx context.Context, username string, ip netip.Addr) bool {
	if s.kv == nil {
		return false
	}
	keys := []string{accountKey(username)}
	if ip.IsValid() {
		keys = append(keys, ipKey(ip))
	}
	for _, k := range keys {
		if _, err := s.kv.Get(ctx, lockPrefix+k); err == nil {
			return true
		} else if !errors.Is(err, cache.ErrMiss) {
			s.logger.Warn("auth: lockout check", "error", err.Error())
		}
	}
	return false
}

// failed counts a failed sign-in and locks when a limit is reached.
func (s *Service) failed(ctx context.Context, p Policy, username string, ip netip.Addr) {
	if s.kv == nil {
		return
	}
	count := func(key string, limit int) {
		if limit <= 0 {
			return
		}
		n, err := s.kv.IncrExpire(ctx, failPrefix+key, p.Window)
		if err != nil {
			s.logger.Warn("auth: lockout count", "error", err.Error())
			return
		}
		if n >= int64(limit) {
			if err := s.kv.Set(ctx, lockPrefix+key, []byte("1"), p.Lockout); err != nil {
				s.logger.Warn("auth: lockout", "error", err.Error())
				return
			}
			_ = s.kv.Del(ctx, failPrefix+key)
			s.logger.Warn("sign-in locked", "key", key, "minutes", p.Lockout.Minutes())
		}
	}
	count(accountKey(username), p.AccountFailures)
	if ip.IsValid() {
		count(ipKey(ip), p.IPFailures)
	}
}

// succeeded clears the counter of the username.
func (s *Service) succeeded(ctx context.Context, username string) {
	if s.kv == nil {
		return
	}
	_ = s.kv.Del(ctx, failPrefix+accountKey(username))
}

// Unlock lifts the lock of a username (operator action).
func (s *Service) Unlock(ctx context.Context, username string) error {
	if s.kv == nil {
		return nil
	}
	if err := s.kv.Del(ctx, lockPrefix+accountKey(username)); err != nil {
		return err
	}
	return s.kv.Del(ctx, failPrefix+accountKey(username))
}

// UnlockAddress lifts the lock of an address (operator action, like Unlock).
func (s *Service) UnlockAddress(ctx context.Context, ip netip.Addr) error {
	if s.kv == nil {
		return nil
	}
	if err := s.kv.Del(ctx, lockPrefix+ipKey(ip)); err != nil {
		return err
	}
	return s.kv.Del(ctx, failPrefix+ipKey(ip))
}
