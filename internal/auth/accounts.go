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
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"group24.de/itsm/internal/db/queries"
)

// --- accounts in the interface (PL77, UI-Plan E63) ---

// InvitationValid is how long the link of an invitation holds.
const InvitationValid = 72 * time.Hour

// ErrInvitationInvalid says the key of an invitation is unknown, used or expired.
var ErrInvitationInvalid = errors.New("auth: the invitation is unknown, used or expired")

// AccountInfo is an account as the administration sees it: where it signs in, when it last
// did, and an open invitation.
type AccountInfo struct {
	Account
	// Origin is "local" (a password of its own) or "oidc" (an identity provider, PL11).
	Origin string
	// Issuer names the identity provider of an account of origin oidc.
	Issuer string
	// LastLoginAt is the last sign-in, local or through the identity provider.
	LastLoginAt *time.Time
	// InvitedUntil is set while an invitation waits to be used.
	InvitedUntil *time.Time
	InvitedEmail string
}

// Accounts lists the accounts of every database with origin, last sign-in and invitation.
func (s *Service) Accounts(ctx context.Context) ([]AccountInfo, error) {
	list, err := s.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := queries.New(s.db.Control()).AccountIdentities(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: identities: %w", err)
	}
	type ident struct {
		issuer string
		last   time.Time
	}
	byAccount := map[uuid.UUID]ident{}
	for _, i := range ids {
		if old, ok := byAccount[i.AccountID]; !ok || i.LastLoginAt.Time.After(old.last) {
			byAccount[i.AccountID] = ident{issuer: i.Issuer, last: i.LastLoginAt.Time}
		}
	}
	type invite struct {
		email string
		until time.Time
	}
	invites := map[uuid.UUID]invite{}
	for _, pool := range s.db.Pools() {
		rows, err := queries.New(pool).ListInvitations(ctx)
		if err != nil {
			return nil, fmt.Errorf("auth: invitations: %w", err)
		}
		for _, r := range rows {
			invites[r.AccountID] = invite{email: r.Email, until: r.ExpiresAt.Time}
		}
	}
	now := s.now()
	out := make([]AccountInfo, 0, len(list))
	for _, a := range list {
		info := AccountInfo{Account: a, Origin: "local", LastLoginAt: a.LastLoginAt}
		if i, ok := byAccount[a.ID]; ok {
			info.Origin, info.Issuer = "oidc", i.issuer
			if info.LastLoginAt == nil || i.last.After(*info.LastLoginAt) {
				last := i.last
				info.LastLoginAt = &last
			}
		}
		if i, ok := invites[a.ID]; ok && now.Before(i.until) {
			until := i.until
			info.InvitedUntil, info.InvitedEmail = &until, i.email
		}
		out = append(out, info)
	}
	return out, nil
}

// AccountByID reads one account of any database.
func (s *Service) AccountByID(ctx context.Context, id uuid.UUID) (Account, error) {
	q, err := s.account(ctx, id)
	if err != nil {
		return Account{}, err
	}
	row, err := q.GetAccount(ctx, id)
	if err != nil {
		return Account{}, notFound(err)
	}
	return fromRow(row), nil
}

// CreateInvitedAccount makes a local account that cannot sign in until its invitation is used:
// its password is random and expired, nobody knows it.
func (s *Service) CreateInvitedAccount(ctx context.Context, tenant, username, displayName string) (Account, error) {
	now := s.now()
	return s.CreateAccountIn(ctx, tenant, username, displayName, randomPassword(), &now)
}

// Invite gives the account a new invitation for the address and returns its one-time key; the
// key itself is kept nowhere, only its hash. A new invitation replaces the one before.
func (s *Service) Invite(ctx context.Context, acc Account, email string, by *uuid.UUID) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("auth: invitation key: %w", err)
	}
	key := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(key))
	q, err := s.account(ctx, acc.ID)
	if err != nil {
		return "", err
	}
	var creator uuid.NullUUID
	if by != nil {
		creator = uuid.NullUUID{UUID: *by, Valid: true}
	}
	if err := q.UpsertInvitation(ctx, queries.UpsertInvitationParams{
		AccountID: acc.ID, TokenHash: hash[:], Email: email,
		ExpiresAt: pgtype.Timestamptz{Time: s.now().Add(InvitationValid).UTC(), Valid: true}, CreatedBy: creator,
	}); err != nil {
		return "", fmt.Errorf("auth: invitation: %w", err)
	}
	return key, nil
}

// AcceptInvitation sets the password of the invited account (the rules of PL28 apply) and
// uses the invitation up; it returns the username.
func (s *Service) AcceptInvitation(ctx context.Context, key, password string) (string, error) {
	hash := sha256.Sum256([]byte(key))
	for _, pool := range s.db.Pools() {
		q := queries.New(pool)
		inv, err := q.GetInvitationByHash(ctx, hash[:])
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("auth: invitation: %w", err)
		}
		if !s.now().Before(inv.ExpiresAt.Time) {
			return "", ErrInvitationInvalid
		}
		if err := s.SetPassword(ctx, inv.Username, password, nil); err != nil {
			return "", err
		}
		if err := q.DeleteInvitation(ctx, inv.AccountID); err != nil {
			return "", fmt.Errorf("auth: invitation: %w", err)
		}
		return inv.Username, nil
	}
	return "", ErrInvitationInvalid
}

// SetOneTimePassword sets a password the person must change at the next sign-in (PL77) and
// drops an open invitation.
func (s *Service) SetOneTimePassword(ctx context.Context, acc Account, password string) error {
	now := s.now()
	if err := s.SetPassword(ctx, acc.Username, password, &now); err != nil {
		return err
	}
	q, err := s.account(ctx, acc.ID)
	if err != nil {
		return err
	}
	if err := q.DeleteInvitation(ctx, acc.ID); err != nil {
		return fmt.Errorf("auth: invitation: %w", err)
	}
	return nil
}

// randomPassword satisfies every rule of PL28 and is never shown.
func randomPassword() string {
	raw := make([]byte, 24)
	_, _ = rand.Read(raw)
	return "Aa1!" + base64.RawURLEncoding.EncodeToString(raw)
}
