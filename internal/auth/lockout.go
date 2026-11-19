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
	"time"

	"group24.de/itsm/internal/cache"
)

// --- lockout after failed sign-ins (PL28, E103) ---
//
// Failed sign-ins count per username and per address in Valkey, each counter living one
// window. When a counter reaches its limit, a lock key holds the username or the address for
// the lockout time; a locked sign-in gets 429 with the time left before the password is
// looked at (AP6). Unknown usernames count too, so that a lock tells nothing about which accounts
// exist. A success clears the counter of the username. Without Valkey there is no lockout.

const (
	failPrefix = "auth:fail:"
	lockPrefix = "auth:lock:"
)

func accountKey(username string) string { return "acct:" + strings.ToLower(username) }

func ipKey(ip netip.Addr) string { return "ip:" + ip.String() }

func clientKey(id string) string { return "client:" + strings.ToLower(id) }

// LockError is a sign-in refused because the name or the address is locked (AP6): it is
// answered with 429 and the time until the lock ends, before any password is checked.
type LockError struct {
	RetryAfter time.Duration
	// Key is what is locked, the name ("acct:…", "client:…") or the address ("ip:…"): the
	// security log sums up the refusals of one lock (AP6).
	Key string
}

func (e *LockError) Error() string { return "auth: sign-in locked" }

// locked returns the lock of the name (its key) or the address, the longer one when both are
// locked; nil when neither is.
func (s *Service) locked(ctx context.Context, key string, ip netip.Addr) *LockError {
	if s.kv == nil {
		return nil
	}
	keys := []string{key}
	if ip.IsValid() {
		keys = append(keys, ipKey(ip))
	}
	var lock *LockError
	for _, k := range keys {
		ttl, err := s.kv.TTL(ctx, lockPrefix+k)
		switch {
		case err == nil:
			if ttl <= 0 {
				ttl = time.Second
			}
			if lock == nil || ttl > lock.RetryAfter {
				lock = &LockError{RetryAfter: ttl, Key: k}
			}
		case !errors.Is(err, cache.ErrMiss):
			s.logger.Warn("auth: lockout check", "error", err.Error())
		}
	}
	return lock
}

// failed counts a failed sign-in of the name (its key) and the address and locks when a limit
// is reached.
func (s *Service) failed(ctx context.Context, p Policy, key string, ip netip.Addr) {
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
	count(key, p.AccountFailures)
	if ip.IsValid() {
		count(ipKey(ip), p.IPFailures)
	}
}

// The lockout of API clients at the token endpoint (AP6): the same counters, limits and
// settings as the sign-in of persons, the address counted together with them.

// ClientLocked returns the lock of the client or the address; nil when neither is locked.
func (s *Service) ClientLocked(ctx context.Context, clientID string, ip netip.Addr) *LockError {
	return s.locked(ctx, clientKey(clientID), ip)
}

// ClientFailed counts a refused token request of the client and the address.
func (s *Service) ClientFailed(ctx context.Context, clientID string, ip netip.Addr) {
	s.failed(ctx, s.Policy(ctx), clientKey(clientID), ip)
}

// ClientSucceeded clears the counter of the client.
func (s *Service) ClientSucceeded(ctx context.Context, clientID string) {
	if s.kv == nil {
		return
	}
	_ = s.kv.Del(ctx, failPrefix+clientKey(clientID))
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
