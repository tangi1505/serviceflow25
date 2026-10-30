// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// settingsOf turns a map into the settings of the root node.
func settingsOf(m map[string]string) Settings {
	return func(_ context.Context, _, key string) (json.RawMessage, string, error) {
		if v, ok := m[key]; ok {
			return json.RawMessage(v), "root", nil
		}
		return nil, "", nil
	}
}

func TestLoadPolicy(t *testing.T) {
	if p := LoadPolicy(context.Background(), nil); p.MinLength != 8 || !p.RequireClasses || p.Sliding != 12*time.Hour ||
		p.MaxLifetime != 24*time.Hour || p.AccountFailures != 5 || p.IPFailures != 20 || p.LocalLogin != LocalLoginAll {
		t.Fatalf("defaults: %+v", p)
	}
	p := LoadPolicy(context.Background(), settingsOf(map[string]string{
		SettingMinLength: "12", SettingRequireClasses: "false", SettingMaxAgeDays: "90", SettingIdleHours: "2",
		SettingMaxHours: "8", SettingAccountFailures: "3", SettingLocalLogin: `"admins"`, SettingBlocklist: `["itsmtest"]`,
	}))
	if p.MinLength != 12 || p.RequireClasses || p.MaxAgeDays != 90 || p.Sliding != 2*time.Hour || p.MaxLifetime != 8*time.Hour ||
		p.AccountFailures != 3 || p.LocalLogin != LocalLoginAdmins || len(p.Blocklist) != 1 {
		t.Fatalf("settings: %+v", p)
	}
	// Invalid values keep the default.
	p = LoadPolicy(context.Background(), settingsOf(map[string]string{SettingMinLength: "2", SettingLocalLogin: `"sometimes"`, SettingIdleHours: `"x"`}))
	if p.MinLength != 8 || p.LocalLogin != LocalLoginAll || p.Sliding != 12*time.Hour {
		t.Fatalf("invalid settings: %+v", p)
	}
}

// The acceptance of PL28, part one: a password too short for the settings is refused; the
// rules name what is wrong.
func TestCheckPasswordWith(t *testing.T) {
	strict := DefaultPolicy()
	long := DefaultPolicy()
	long.MinLength, long.RequireClasses, long.Blocklist = 12, false, []string{"itsmtest"}
	for _, c := range []struct {
		p        Policy
		user, pw string
		rule     string
	}{
		{strict, "anna", "Kurz-1!", "at least 8 characters"},
		{strict, "anna", "langesKennwort", "an upper-case letter"},
		{strict, "anna", "Passwort123!", "not a common password"},
		{strict, "anna", "!Kennwort2026", "not a common password"},
		{strict, "anna", "Anna-Mai-2026!", "not the username"},
		{long, "anna", "Sicher-1!", "at least 12 characters"},
		{long, "anna", "Itsmtest99999", "not a common password"},
	} {
		err := CheckPasswordWith(c.p, c.user, c.pw)
		if !errors.Is(err, ErrWeakPassword) || !strings.Contains(err.Error(), c.rule) {
			t.Errorf("%q: %v, want %q", c.pw, err, c.rule)
		}
	}
	for _, c := range []struct {
		p        Policy
		user, pw string
	}{
		{strict, "anna", "Wolke-Tisch-7!"},
		{long, "anna", "wolkentischlampe"},
	} {
		if err := CheckPasswordWith(c.p, c.user, c.pw); err != nil {
			t.Errorf("%q: %v", c.pw, err)
		}
	}
}

func TestSessionLifetimesFromSettings(t *testing.T) {
	created := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	s := Session{CreatedAt: created, RefreshedAt: created, Sliding: time.Hour, MaxAge: 90 * time.Minute}
	if got := s.ExpiresAt(); !got.Equal(created.Add(time.Hour)) {
		t.Fatalf("sliding from the settings: %v", got)
	}
	s.RefreshedAt = created.Add(80 * time.Minute)
	if got := s.ExpiresAt(); !got.Equal(created.Add(90 * time.Minute)) {
		t.Fatalf("capped by the settings: %v", got)
	}
}

// Lockout, local sign-in, password age and session lifetimes against PostgreSQL and Valkey.
func TestSecuritySettings(t *testing.T) {
	svc, kv := testService(t)
	ctx := t.Context()
	settings := map[string]string{SettingAccountFailures: "3", SettingIPFailures: "5", SettingMaxAgeDays: "30", SettingIdleHours: "2"}
	svc.Settings = settingsOf(settings)
	svc.ForgetPolicy()
	suffix := fmt.Sprint(time.Now().UnixNano() % 1000000)
	user := "authtest_lock" + suffix
	acc, err := svc.CreateAccount(ctx, user, "Lock", "Wolke-Tisch-7!", nil)
	if err != nil {
		t.Fatal(err)
	}
	// The password age of the settings gives the expiry.
	if acc.PasswordExpiresAt == nil || time.Until(*acc.PasswordExpiresAt) < 29*24*time.Hour {
		t.Fatalf("expiry from max_age_days: %v", acc.PasswordExpiresAt)
	}
	ip := netip.MustParseAddr("192.0.2." + fmt.Sprint(time.Now().UnixNano()%200+1))
	t.Cleanup(func() {
		for _, k := range []string{accountKey(user), ipKey(ip)} {
			_ = kv.Del(context.Background(), lockPrefix+k)
			_ = kv.Del(context.Background(), failPrefix+k)
		}
	})
	for i := range 3 {
		if _, err := svc.Login(ctx, user, "falsch", ip); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	// Locked: even the right password is refused, with the reason locked.
	var le *LoginError
	if _, err := svc.Login(ctx, user, "Wolke-Tisch-7!", ip); !errors.As(err, &le) || le.Reason != "locked" {
		t.Fatalf("locked account: %v", err)
	}
	if err := svc.Unlock(ctx, user); err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Login(ctx, user, "Wolke-Tisch-7!", ip)
	if err != nil {
		t.Fatalf("after unlock: %v", err)
	}
	// The session takes the lifetime of the settings.
	if sess.Sliding != 2*time.Hour {
		t.Fatalf("session lifetime: %v", sess.Sliding)
	}
	ttl, err := kv.TTL(ctx, keyPrefix+sess.ID)
	if err != nil || ttl > 2*time.Hour || ttl < 2*time.Hour-time.Minute {
		t.Fatalf("session TTL: %v %v", ttl, err)
	}

	// The address is locked after five failures, also for other usernames.
	for i := range 5 {
		_, _ = svc.Login(ctx, fmt.Sprintf("authtest_nobody%d", i), "x", ip)
	}
	if _, err := svc.Login(ctx, user, "Wolke-Tisch-7!", ip); !errors.As(err, &le) || le.Reason != "locked" {
		t.Fatalf("locked address: %v", err)
	}
	if _, err := svc.Login(ctx, user, "Wolke-Tisch-7!", netip.Addr{}); err != nil {
		t.Fatalf("another address: %v", err)
	}

	// Local sign-in: none refuses everyone, admins only who may manage rights.
	settings[SettingLocalLogin] = `"none"`
	svc.ForgetPolicy()
	if _, err := svc.Login(ctx, user, "Wolke-Tisch-7!", netip.Addr{}); !errors.As(err, &le) || le.Reason != "local_login_disabled" {
		t.Fatalf("local login none: %v", err)
	}
	settings[SettingLocalLogin] = `"admins"`
	svc.ForgetPolicy()
	svc.AllowLocal = func(_ context.Context, id uuid.UUID) (bool, error) { return id == acc.ID, nil }
	if _, err := svc.Login(ctx, user, "Wolke-Tisch-7!", netip.Addr{}); err != nil {
		t.Fatalf("local login admins for an admin: %v", err)
	}
	svc.AllowLocal = func(context.Context, uuid.UUID) (bool, error) { return false, nil }
	if _, err := svc.Login(ctx, user, "Wolke-Tisch-7!", netip.Addr{}); !errors.As(err, &le) || le.Reason != "local_login_disabled" {
		t.Fatalf("local login admins for another: %v", err)
	}
}
