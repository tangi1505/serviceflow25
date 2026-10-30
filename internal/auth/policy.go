// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package auth

import (
	"bufio"
	"context"
	_ "embed" // the list of common passwords
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// --- security settings of local accounts (Hauptplan E72, E73, E103; PL28) ---
//
// The rules live as settings of the root node and fall back to E72 and E73: password at least
// 8 characters with classes, no expiry; sessions 12 hours sliding and at most 24 hours; 5
// failed sign-ins of an account or 20 of an address within 15 minutes lock it for 15 minutes;
// local sign-in for every account. Always: not a common password and not the username.
// Two-factor sign-in is the identity provider's (Keycloak), not the core's.

// Setting keys (root node).
const (
	SettingMinLength       = "security.password.min_length"
	SettingRequireClasses  = "security.password.require_classes"
	SettingMaxAgeDays      = "security.password.max_age_days"
	SettingBlocklist       = "security.password.blocklist"
	SettingIdleHours       = "security.session.idle_hours"
	SettingMaxHours        = "security.session.max_hours"
	SettingAccountFailures = "security.lockout.account_failures"
	SettingIPFailures      = "security.lockout.ip_failures"
	SettingWindowMinutes   = "security.lockout.window_minutes"
	SettingLockoutMinutes  = "security.lockout.minutes"
	SettingLocalLogin      = "security.local_login"
)

// Values of security.local_login.
const (
	LocalLoginAll    = "all"    // every local account
	LocalLoginAdmins = "admins" // only accounts that may manage rights, as an emergency access
	LocalLoginNone   = "none"   // no local sign-in; the identity provider only
)

// Policy is the effective security settings.
type Policy struct {
	MinLength       int           `json:"min_length"`
	RequireClasses  bool          `json:"require_classes"`
	MaxAgeDays      int           `json:"max_age_days"`
	Blocklist       []string      `json:"-"`
	Sliding         time.Duration `json:"-"`
	MaxLifetime     time.Duration `json:"-"`
	AccountFailures int           `json:"-"`
	IPFailures      int           `json:"-"`
	Window          time.Duration `json:"-"`
	Lockout         time.Duration `json:"-"`
	LocalLogin      string        `json:"local_login"`
}

// DefaultPolicy is the policy without settings (E72, E73).
func DefaultPolicy() Policy {
	return Policy{MinLength: MinPasswordLen, RequireClasses: true, Sliding: SlidingLifetime, MaxLifetime: MaxLifetime,
		AccountFailures: 5, IPFailures: 20, Window: 15 * time.Minute, Lockout: 15 * time.Minute, LocalLogin: LocalLoginAll}
}

// Settings reads a setting of a node (org.Service.Setting).
type Settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)

// LoadPolicy reads the settings of the root node; a missing or invalid value keeps the
// default.
func LoadPolicy(ctx context.Context, settings Settings) Policy {
	p := DefaultPolicy()
	if settings == nil {
		return p
	}
	get := func(key string, into any) bool {
		raw, _, err := settings(ctx, "root", key)
		return err == nil && raw != nil && json.Unmarshal(raw, into) == nil
	}
	var n int
	var b bool
	var s string
	var list []string
	if get(SettingMinLength, &n) && n >= 4 && n <= 128 {
		p.MinLength = n
	}
	if get(SettingRequireClasses, &b) {
		p.RequireClasses = b
	}
	if get(SettingMaxAgeDays, &n) && n >= 0 {
		p.MaxAgeDays = n
	}
	if get(SettingBlocklist, &list) {
		p.Blocklist = list
	}
	if get(SettingIdleHours, &n) && n >= 1 {
		p.Sliding = time.Duration(n) * time.Hour
	}
	if get(SettingMaxHours, &n) && n >= 1 {
		p.MaxLifetime = time.Duration(n) * time.Hour
	}
	if get(SettingAccountFailures, &n) && n >= 0 {
		p.AccountFailures = n
	}
	if get(SettingIPFailures, &n) && n >= 0 {
		p.IPFailures = n
	}
	if get(SettingWindowMinutes, &n) && n >= 1 {
		p.Window = time.Duration(n) * time.Minute
	}
	if get(SettingLockoutMinutes, &n) && n >= 1 {
		p.Lockout = time.Duration(n) * time.Minute
	}
	if get(SettingLocalLogin, &s) && (s == LocalLoginAll || s == LocalLoginAdmins || s == LocalLoginNone) {
		p.LocalLogin = s
	}
	return p
}

// policyCache keeps the policy for a while: the session store asks on every request.
type policyCache struct {
	mu    sync.Mutex
	at    time.Time
	value Policy
}

const policyTTL = 30 * time.Second

func (c *policyCache) get(ctx context.Context, settings Settings, now time.Time) Policy {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.at.IsZero() || now.Sub(c.at) > policyTTL {
		c.value, c.at = LoadPolicy(ctx, settings), now
	}
	return c.value
}

// WeakPasswordError lists the rules a password breaks; it is an ErrWeakPassword.
type WeakPasswordError struct {
	Rules []string
}

func (e *WeakPasswordError) Error() string {
	return "password breaks the rules: " + strings.Join(e.Rules, "; ")
}

// Is makes a WeakPasswordError an ErrWeakPassword.
func (e *WeakPasswordError) Is(target error) bool { return target == ErrWeakPassword }

// CheckPasswordWith applies a policy to a new password of an account.
func CheckPasswordWith(p Policy, username, pw string) error {
	var rules []string
	var upper, lower, digit, special bool
	n := 0
	for _, r := range pw {
		n++
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		default:
			special = true
		}
	}
	if n < p.MinLength {
		rules = append(rules, "at least "+strconv.Itoa(p.MinLength)+" characters")
	}
	if p.RequireClasses && (!upper || !lower || !digit || !special) {
		rules = append(rules, "an upper-case letter, a lower-case letter, a digit and a special character")
	}
	lowerPw := strings.ToLower(pw)
	if u := strings.ToLower(username); len(u) >= 3 && strings.Contains(lowerPw, u) {
		rules = append(rules, "not the username")
	}
	if isCommon(lowerPw, p.Blocklist) {
		rules = append(rules, "not a common password")
	}
	if len(rules) > 0 {
		return &WeakPasswordError{Rules: rules}
	}
	return nil
}

//go:embed common_passwords.txt
var commonList string

var common = func() map[string]bool {
	m := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(commonList))
	for sc.Scan() {
		if w := strings.TrimSpace(sc.Text()); w != "" && !strings.HasPrefix(w, "#") {
			m[strings.ToLower(w)] = true
		}
	}
	return m
}()

// isCommon tells whether a password, in lower case, is a common one: as it is, or its core
// with the digits and special characters at the end taken off ("Passwort123!" is "passwort").
func isCommon(lowerPw string, extra []string) bool {
	core := strings.TrimRightFunc(lowerPw, func(r rune) bool { return !unicode.IsLetter(r) })
	core = strings.TrimLeftFunc(core, func(r rune) bool { return !unicode.IsLetter(r) })
	for _, w := range []string{lowerPw, core} {
		if w == "" {
			continue
		}
		if common[w] {
			return true
		}
		for _, x := range extra {
			if strings.EqualFold(w, x) {
				return true
			}
		}
	}
	return false
}
