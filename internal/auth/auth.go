// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package auth signs local accounts in (Plattform-Kern PL5; Hauptplan E10, E72 to E75, E77):
// accounts with Argon2id hashes in PostgreSQL, sessions in Valkey with a time to live, a cookie
// in the browser. LDAP and OIDC come with PL9 and PL10 and reuse the sessions.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db/queries"
)

var (
	// ErrInvalidCredentials covers unknown name, wrong password and disabled account alike,
	// so that the answer does not tell which accounts exist; the log tells the operator.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrAccountExists says the username is taken.
	ErrAccountExists = errors.New("auth: account exists")
	// ErrAccountNotFound says there is no such account.
	ErrAccountNotFound = errors.New("auth: account not found")
	// ErrInvalidUsername says the name is not snake_case (E75).
	ErrInvalidUsername = errors.New("auth: username must be snake_case: a letter, then letters, digits or _ (at most 63)")
)

// LoginError is a refused sign-in. Towards the caller it is ErrInvalidCredentials; the reason
// and the account, when it exists, go to the security event log (PL27).
type LoginError struct {
	Reason  string     // unknown_account, no_local_password, wrong_password, account_disabled
	Account *uuid.UUID // nil for an unknown account
}

func (e *LoginError) Error() string { return "auth: invalid credentials: " + e.Reason }

// Is makes a LoginError an ErrInvalidCredentials.
func (e *LoginError) Is(target error) bool { return target == ErrInvalidCredentials }

func refused(reason string, account *uuid.UUID) error {
	return &LoginError{Reason: reason, Account: account}
}

var usernameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Account is a local account without its hash.
type Account struct {
	ID                uuid.UUID
	Username          string
	DisplayName       string
	PasswordChangedAt time.Time
	PasswordExpiresAt *time.Time
	DisabledAt        *time.Time
	CreatedAt         time.Time
	// Tenant is the tenant the account belongs to (PL44); empty for the installation.
	Tenant string
}

// PasswordExpired reports whether the password must be changed before anything else (E77).
func (a Account) PasswordExpired(now time.Time) bool {
	return a.PasswordExpiresAt != nil && !now.Before(*a.PasswordExpiresAt)
}

// Service manages accounts and sessions.
type Service struct {
	q        *queries.Queries
	kv       *cache.Client
	sessions *Sessions
	logger   *slog.Logger
	now      func() time.Time
	policy   policyCache
	// Settings reads the security settings of the root node (PL28); nil means the defaults.
	Settings Settings
	// AllowLocal tells whether an account may sign in locally while security.local_login is
	// "admins" (it may manage rights); nil allows none then.
	AllowLocal func(ctx context.Context, account uuid.UUID) (bool, error)
}

// New returns a service on the database pool and the Valkey client. Without Valkey (the
// command line) there are no sessions and no lockout.
func New(pool *pgxpool.Pool, kv *cache.Client, logger *slog.Logger) *Service {
	s := &Service{q: queries.New(pool), kv: kv, sessions: NewSessions(kv), logger: logger, now: time.Now}
	s.sessions.Lifetimes = func(ctx context.Context) (time.Duration, time.Duration) {
		p := s.Policy(ctx)
		return p.Sliding, p.MaxLifetime
	}
	return s
}

// Policy is the effective security settings (cached for 30 seconds).
func (s *Service) Policy(ctx context.Context) Policy { return s.policy.get(ctx, s.Settings, s.now()) }

// ForgetPolicy drops the cached settings (tests, and after a change through the API).
func (s *Service) ForgetPolicy() {
	s.policy.mu.Lock()
	s.policy.at = time.Time{}
	s.policy.mu.Unlock()
}

// passwordRules checks a new password and returns the expiry the settings give it.
func (s *Service) passwordRules(ctx context.Context, username, password string, expires *time.Time) (*time.Time, error) {
	p := s.Policy(ctx)
	if err := CheckPasswordWith(p, username, password); err != nil {
		return nil, err
	}
	if expires == nil && p.MaxAgeDays > 0 {
		at := s.now().UTC().Add(time.Duration(p.MaxAgeDays) * 24 * time.Hour)
		expires = &at
	}
	return expires, nil
}

// Sessions gives access to the session store (for the HTTP layer).
func (s *Service) Sessions() *Sessions { return s.sessions }

// CreateAccount makes a local account; the password must satisfy CheckPassword.
func (s *Service) CreateAccount(ctx context.Context, username, displayName, password string, expires *time.Time) (Account, error) {
	if !usernameRe.MatchString(username) {
		return Account{}, ErrInvalidUsername
	}
	if displayName == "" {
		displayName = username
	}
	expires, err := s.passwordRules(ctx, username, password, expires)
	if err != nil {
		return Account{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Account{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Account{}, fmt.Errorf("auth: id: %w", err)
	}
	row, err := s.q.CreateAccount(ctx, queries.CreateAccountParams{
		ID: id, Username: username, DisplayName: displayName, PasswordHash: hash, PasswordExpiresAt: ts(expires),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Account{}, ErrAccountExists
		}
		return Account{}, fmt.Errorf("auth: create account: %w", err)
	}
	return fromRow(row), nil
}

// SetPassword replaces the password of an account (operator action, no check of the old one).
func (s *Service) SetPassword(ctx context.Context, username, password string, expires *time.Time) error {
	expires, err := s.passwordRules(ctx, username, password, expires)
	if err != nil {
		return err
	}
	row, err := s.q.GetAccountByUsername(ctx, username)
	if err != nil {
		return notFound(err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if err := s.q.SetAccountPassword(ctx, queries.SetAccountPasswordParams{ID: row.ID, PasswordHash: hash, PasswordExpiresAt: ts(expires)}); err != nil {
		return fmt.Errorf("auth: set password: %w", err)
	}
	return nil
}

// ChangePassword is the user's own change: the current password must match, the new one must
// satisfy the rules, and the expiry is cleared. The session is updated so that the block of E77
// ends at once.
func (s *Service) ChangePassword(ctx context.Context, session Session, current, next string) error {
	row, err := s.q.GetAccount(ctx, session.AccountID)
	if err != nil {
		return notFound(err)
	}
	if row.PasswordHash == "" {
		return ErrInvalidCredentials
	}
	ok, err := VerifyPassword(row.PasswordHash, current)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCredentials
	}
	expires, err := s.passwordRules(ctx, row.Username, next, nil)
	if err != nil {
		return err
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if err := s.q.SetAccountPassword(ctx, queries.SetAccountPasswordParams{ID: row.ID, PasswordHash: hash, PasswordExpiresAt: ts(expires)}); err != nil {
		return fmt.Errorf("auth: change password: %w", err)
	}
	if session.ID != "" && session.PasswordExpired {
		session.PasswordExpired = false
		if err := s.sessions.Update(ctx, session); err != nil {
			return err
		}
	}
	return nil
}

// SetDisabled blocks or unblocks an account; a blocked account cannot sign in, running sessions
// end at their TTL.
func (s *Service) SetDisabled(ctx context.Context, username string, disabled bool) error {
	row, err := s.q.GetAccountByUsername(ctx, username)
	if err != nil {
		return notFound(err)
	}
	var at pgtype.Timestamptz
	if disabled {
		at = pgtype.Timestamptz{Time: s.now().UTC(), Valid: true}
	}
	if err := s.q.SetAccountDisabled(ctx, queries.SetAccountDisabledParams{ID: row.ID, DisabledAt: at}); err != nil {
		return fmt.Errorf("auth: set disabled: %w", err)
	}
	return nil
}

// ListAccounts returns all local accounts by name.
func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.q.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: list accounts: %w", err)
	}
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

// Login checks the credentials and opens a session. Every failure is ErrInvalidCredentials
// towards the caller; the reason goes to the log. ip is the caller's address for the lockout
// (PL28); an invalid one counts for the account only.
func (s *Service) Login(ctx context.Context, username, password string, ip netip.Addr) (Session, error) {
	p := s.Policy(ctx)
	if s.locked(ctx, username, ip) {
		s.logger.Info("login refused", "username", username, "reason", "locked")
		_, _ = HashPassword(password)
		return Session{}, refused("locked", nil)
	}
	sess, err := s.login(ctx, p, username, password)
	var le *LoginError
	if errors.As(err, &le) && le.Reason != "local_login_disabled" {
		s.failed(ctx, p, username, ip)
	} else if err == nil {
		s.succeeded(ctx, username)
	}
	return sess, err
}

func (s *Service) login(ctx context.Context, p Policy, username, password string) (Session, error) {
	row, err := s.q.GetAccountByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Info("login refused", "username", username, "reason", "unknown account")
			// Hash anyway so that the timing does not tell whether the account exists.
			_, _ = HashPassword(password)
			return Session{}, refused("unknown_account", nil)
		}
		return Session{}, fmt.Errorf("auth: login: %w", err)
	}
	if row.PasswordHash == "" {
		// An account from an identity provider (PL11) has no local password.
		s.logger.Info("login refused", "username", username, "reason", "no local password")
		_, _ = HashPassword(password)
		return Session{}, refused("no_local_password", &row.ID)
	}
	ok, err := VerifyPassword(row.PasswordHash, password)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		s.logger.Info("login refused", "username", username, "reason", "wrong password")
		return Session{}, refused("wrong_password", &row.ID)
	}
	if row.DisabledAt.Valid {
		s.logger.Info("login refused", "username", username, "reason", "account disabled")
		return Session{}, refused("account_disabled", &row.ID)
	}
	// Local sign-in may be limited to the identity provider (PL28, E103).
	switch p.LocalLogin {
	case LocalLoginNone:
		return Session{}, refused("local_login_disabled", &row.ID)
	case LocalLoginAdmins:
		ok := false
		if s.AllowLocal != nil {
			if ok, err = s.AllowLocal(ctx, row.ID); err != nil {
				return Session{}, err
			}
		}
		if !ok {
			return Session{}, refused("local_login_disabled", &row.ID)
		}
	}
	acc := fromRow(row)
	sess, err := s.sessions.Create(ctx, Session{
		AccountID: acc.ID, Username: acc.Username, DisplayName: acc.DisplayName,
		PasswordExpired: acc.PasswordExpired(s.now()),
	})
	if err != nil {
		return Session{}, err
	}
	s.logger.Info("login", "username", username, "password_expired", sess.PasswordExpired)
	return sess, nil
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}

func fromRow(r queries.Account) Account {
	a := Account{ID: r.ID, Username: r.Username, DisplayName: r.DisplayName,
		PasswordChangedAt: r.PasswordChangedAt.Time, CreatedAt: r.CreatedAt.Time}
	if r.PasswordExpiresAt.Valid {
		t := r.PasswordExpiresAt.Time
		a.PasswordExpiresAt = &t
	}
	if r.DisabledAt.Valid {
		t := r.DisabledAt.Time
		a.DisabledAt = &t
	}
	a.Tenant = r.Tenant.String
	return a
}

func ts(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAccountNotFound
	}
	return fmt.Errorf("auth: %w", err)
}
