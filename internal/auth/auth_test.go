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
	"io"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db"
)

func TestCheckPasswordRules(t *testing.T) {
	for _, pw := range []string{"Secret-1!", "Aa1!aaaa", "Pässwörd-9"} {
		if err := CheckPassword(pw); err != nil {
			t.Errorf("%q should pass: %v", pw, err)
		}
	}
	for _, pw := range []string{"", "Short-1", "alllower-1!", "ALLUPPER-1!", "NoDigits-!", "NoSpecial12", "aaaaaaaaaaaaaaaa"} {
		if err := CheckPassword(pw); !errors.Is(err, ErrWeakPassword) {
			t.Errorf("%q should fail, got %v", pw, err)
		}
	}
}

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("Secret-1!")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected hash format: %s", h)
	}
	if ok, err := VerifyPassword(h, "Secret-1!"); err != nil || !ok {
		t.Fatalf("right password: ok=%v err=%v", ok, err)
	}
	if ok, err := VerifyPassword(h, "Secret-2!"); err != nil || ok {
		t.Fatalf("wrong password: ok=%v err=%v", ok, err)
	}
	h2, _ := HashPassword("Secret-1!")
	if h2 == h {
		t.Fatal("two hashes of the same password must differ (salt)")
	}
	if _, err := VerifyPassword("$bcrypt$nope", "x"); err == nil {
		t.Fatal("foreign hash must be refused")
	}
}

func TestSessionExpiry(t *testing.T) {
	created := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	s := Session{CreatedAt: created, RefreshedAt: created}
	if got, want := s.ExpiresAt(), created.Add(SlidingLifetime); !got.Equal(want) {
		t.Fatalf("fresh session: %v, want %v", got, want)
	}
	s.RefreshedAt = created.Add(20 * time.Hour)
	if got, want := s.ExpiresAt(), created.Add(MaxLifetime); !got.Equal(want) {
		t.Fatalf("late refresh is capped: %v, want %v", got, want)
	}
}

func testService(t *testing.T) (*Service, *cache.Client) {
	t.Helper()
	url, addr := os.Getenv("ITSM_TEST_DATABASE_URL"), os.Getenv("ITSM_TEST_VALKEY_ADDR")
	if url == "" || addr == "" {
		t.Skip("ITSM_TEST_DATABASE_URL or ITSM_TEST_VALKEY_ADDR not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	kv, err := cache.Open(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(kv.Close)
	if _, err := pool.Exec(ctx, "DELETE FROM account WHERE username LIKE 'authtest%'"); err != nil {
		t.Fatal(err)
	}
	return New(db.Single(pool), kv, quiet), kv
}

func TestAccountsAndLogin(t *testing.T) {
	svc, kv := testService(t)
	ctx := t.Context()

	if _, err := svc.CreateAccount(ctx, "Auth-Test", "x", "Secret-1!", nil); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("bad username: %v", err)
	}
	if _, err := svc.CreateAccount(ctx, "authtest", "x", "weak", nil); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak password: %v", err)
	}
	acc, err := svc.CreateAccount(ctx, "authtest", "Auth Test", "Secret-1!", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateAccount(ctx, "authtest", "again", "Secret-1!", nil); !errors.Is(err, ErrAccountExists) {
		t.Fatalf("duplicate: %v", err)
	}

	if _, err := svc.Login(ctx, "authtest", "wrong-Password1!", netip.Addr{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := svc.Login(ctx, "authtest_nobody", "Secret-1!", netip.Addr{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown account: %v", err)
	}
	sess, err := svc.Login(ctx, "authtest", "Secret-1!", netip.Addr{})
	if err != nil {
		t.Fatal(err)
	}
	if sess.AccountID != acc.ID || sess.ID == "" || sess.PasswordExpired {
		t.Fatalf("session: %+v", sess)
	}
	// The key in Valkey carries the TTL (PL5 Prüfweg): 12 h sliding, within a second of it.
	ttl, err := kv.TTL(ctx, keyPrefix+sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ttl < SlidingLifetime-5*time.Second || ttl > SlidingLifetime {
		t.Fatalf("ttl %v, want about %v", ttl, SlidingLifetime)
	}
	got, err := svc.Sessions().Get(ctx, sess.ID)
	if err != nil || got.Username != "authtest" {
		t.Fatalf("get session: %+v %v", got, err)
	}
	if err := svc.Logout(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sessions().Get(ctx, sess.ID); !errors.Is(err, ErrNoSession) {
		t.Fatalf("after logout: %v", err)
	}

	// Disabled accounts cannot sign in; the error is the same as for a wrong password.
	if err := svc.SetDisabled(ctx, "authtest", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, "authtest", "Secret-1!", netip.Addr{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled: %v", err)
	}
	if err := svc.SetDisabled(ctx, "authtest", false); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDisabled(ctx, "authtest_nobody", true); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("disable unknown: %v", err)
	}

	// Expired password: the session says so until the user changes the password (E77).
	past := time.Now().Add(-time.Hour)
	if err := svc.SetPassword(ctx, "authtest", "Secret-2!", &past); err != nil {
		t.Fatal(err)
	}
	sess, err = svc.Login(ctx, "authtest", "Secret-2!", netip.Addr{})
	if err != nil || !sess.PasswordExpired {
		t.Fatalf("expired login: %+v %v", sess, err)
	}
	if err := svc.ChangePassword(ctx, sess, "Secret-2!", "weak"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak new password: %v", err)
	}
	if err := svc.ChangePassword(ctx, sess, "Secret-9!", "Secret-3!"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong current password: %v", err)
	}
	if err := svc.ChangePassword(ctx, sess, "Secret-2!", "Secret-3!"); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Sessions().Get(ctx, sess.ID)
	if err != nil || got.PasswordExpired {
		t.Fatalf("after change: %+v %v", got, err)
	}
	if _, err := svc.Login(ctx, "authtest", "Secret-3!", netip.Addr{}); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
	accs, err := svc.ListAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range accs {
		if a.Username == "authtest" && a.PasswordExpiresAt == nil && a.DisabledAt == nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("account not listed as expected: %+v", accs)
	}
}

func TestSessionSlidingRefresh(t *testing.T) {
	_, kv := testService(t)
	ctx := t.Context()
	st := NewSessions(kv)
	now := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return now }
	sess, err := st.Create(ctx, Session{Username: "authtest_slide"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Delete(context.Background(), sess.ID) })

	// Within a minute nothing is written; after it the refresh moves the expiry.
	now = now.Add(30 * time.Second)
	got, err := st.Get(ctx, sess.ID)
	if err != nil || !got.RefreshedAt.Equal(sess.RefreshedAt) {
		t.Fatalf("early refresh: %+v %v", got, err)
	}
	now = now.Add(2 * time.Minute)
	if got, err = st.Get(ctx, sess.ID); err != nil || !got.RefreshedAt.Equal(now) {
		t.Fatalf("refresh after a minute: %+v %v", got, err)
	}
	// Past the absolute lifetime the session is gone even when the key still exists.
	now = sess.CreatedAt.Add(MaxLifetime + time.Second)
	if _, err := st.Get(ctx, sess.ID); !errors.Is(err, ErrNoSession) {
		t.Fatalf("after 24 h: %v", err)
	}
	if _, err := st.Get(ctx, ""); !errors.Is(err, ErrNoSession) {
		t.Fatalf("empty id: %v", err)
	}
}
