// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/cache"
)

// Session lifetimes (Hauptplan E72): sliding 12 hours, at most 24 hours since sign-in, the
// defaults of the settings security.session.idle_hours and max_hours (PL28).
const (
	SlidingLifetime = 12 * time.Hour
	MaxLifetime     = 24 * time.Hour
	// refreshEvery limits the writes to Valkey: a session is extended at most once a minute,
	// so a read of a record costs one GET and no SET (LZ1).
	refreshEvery = time.Minute
	keyPrefix    = "session:"
)

// ErrNoSession says the cookie names no live session.
var ErrNoSession = errors.New("auth: no such session")

// Session is what the cookie points to in Valkey.
type Session struct {
	ID              string    `json:"-"`
	AccountID       uuid.UUID `json:"account_id"`
	Username        string    `json:"username"`
	DisplayName     string    `json:"display_name"`
	PasswordExpired bool      `json:"password_expired"`
	// ActiveRole and ActiveOrg narrow the session to one role and one node (PL10); empty
	// means everything the person holds.
	ActiveRole string `json:"active_role,omitempty"`
	ActiveOrg  string `json:"active_org,omitempty"`
	// Tenant is the tenant the session works in (PL44); empty until the first request picks
	// one.
	Tenant string `json:"tenant,omitempty"`
	// Entered is the tenant whose mirror of the account this session last brought up to date
	// (PL47b): a new session and every switch refresh it.
	Entered     string    `json:"entered,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	RefreshedAt time.Time `json:"refreshed_at"`
	// Sliding and MaxAge are the lifetimes of the settings when the session was last used
	// (PL28); zero means the defaults.
	Sliding time.Duration `json:"sliding,omitempty"`
	MaxAge  time.Duration `json:"max_age,omitempty"`
}

// ExpiresAt is when the session ends unless it is used again before: the earlier of sliding
// and absolute lifetime.
func (s Session) ExpiresAt() time.Time {
	return s.expiresFrom(s.RefreshedAt)
}

func (s Session) expiresFrom(now time.Time) time.Time {
	slide, maxAge := s.Sliding, s.MaxAge
	if slide <= 0 {
		slide = SlidingLifetime
	}
	if maxAge <= 0 {
		maxAge = MaxLifetime
	}
	sliding := now.Add(slide)
	if hard := s.CreatedAt.Add(maxAge); hard.Before(sliding) {
		return hard
	}
	return sliding
}

// Sessions stores sessions in Valkey; the key carries the TTL, so sessions clean up themselves.
type Sessions struct {
	kv  *cache.Client
	now func() time.Time
	// Lifetimes gives the sliding and the absolute lifetime of the settings (PL28); nil means
	// the defaults.
	Lifetimes func(ctx context.Context) (sliding, maxAge time.Duration)
}

// lifetimes puts the current settings into a session; it reports whether they changed.
func (st *Sessions) lifetimes(ctx context.Context, s *Session) bool {
	if st.Lifetimes == nil {
		return false
	}
	slide, maxAge := st.Lifetimes(ctx)
	if slide == s.Sliding && maxAge == s.MaxAge {
		return false
	}
	s.Sliding, s.MaxAge = slide, maxAge
	return true
}

// NewSessions returns a store on the given Valkey client.
func NewSessions(kv *cache.Client) *Sessions {
	return &Sessions{kv: kv, now: time.Now}
}

// Create stores a new session and returns it with its id.
func (st *Sessions) Create(ctx context.Context, s Session) (Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, fmt.Errorf("auth: session id: %w", err)
	}
	s.ID = base64.RawURLEncoding.EncodeToString(raw)
	now := st.now().UTC()
	s.CreatedAt, s.RefreshedAt = now, now
	st.lifetimes(ctx, &s)
	if err := st.write(ctx, s, now); err != nil {
		return Session{}, err
	}
	return s, nil
}

// Get loads a session and extends it (sliding lifetime), at most once a minute.
func (st *Sessions) Get(ctx context.Context, id string) (Session, error) {
	if id == "" {
		return Session{}, ErrNoSession
	}
	b, err := st.kv.Get(ctx, keyPrefix+id)
	if errors.Is(err, cache.ErrMiss) {
		return Session{}, ErrNoSession
	}
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return Session{}, fmt.Errorf("auth: session %s: %w", id, err)
	}
	s.ID = id
	now := st.now().UTC()
	// Changed settings apply on the next use (PL28).
	changed := st.lifetimes(ctx, &s)
	if !now.Before(s.expiresFrom(s.RefreshedAt)) {
		_ = st.kv.Del(ctx, keyPrefix+id)
		return Session{}, ErrNoSession
	}
	if changed || now.Sub(s.RefreshedAt) >= refreshEvery {
		s.RefreshedAt = now
		if err := st.write(ctx, s, now); err != nil {
			return Session{}, err
		}
	}
	return s, nil
}

// Update rewrites a session (after a password change) and keeps its expiry.
func (st *Sessions) Update(ctx context.Context, s Session) error {
	return st.write(ctx, s, st.now().UTC())
}

// Delete ends a session; a missing session is no error.
func (st *Sessions) Delete(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	return st.kv.Del(ctx, keyPrefix+id)
}

func (st *Sessions) write(ctx context.Context, s Session, now time.Time) error {
	ttl := s.expiresFrom(now).Sub(now)
	if ttl <= 0 {
		return ErrNoSession
	}
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("auth: session: %w", err)
	}
	return st.kv.Set(ctx, keyPrefix+s.ID, b, ttl)
}

type ctxKey struct{}

// WithSession puts the session of the request into the context.
func WithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// SessionFrom returns the session of the request, if any.
func SessionFrom(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(Session)
	return s, ok
}
