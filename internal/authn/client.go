// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
)

// --- API clients (PL13, G187, G226, E90) ---
//
// A client is a machine: it gets a token with client id and secret (OAuth 2 client
// credentials, RFC 6749 §4.4) and calls the API with `Authorization: Bearer`. The token is an
// opaque random value in Valkey with an expiry; it can be revoked at any time. A client
// belongs to an account without a local password, whose person holds the roles.

// Settings of the root node that give the defaults (G179).
const (
	SettingRateLimit = "api.rate_limit" // requests per minute, default 600
	SettingTokenTTL  = "api.token_ttl"  //nolint:gosec // G101: a setting key, no credential; seconds, default 3600
)

const (
	defaultRateLimit = 600
	defaultTokenTTL  = 3600
	tokenPrefix      = "token:"
	ratePrefix       = "rate:"
)

var (
	// ErrInvalidClient says client id or secret are wrong, or the client is disabled (401).
	ErrInvalidClient = errors.New("authn: invalid client")
	// ErrInvalidToken says the bearer token is unknown or expired (401).
	ErrInvalidToken = errors.New("authn: invalid token")
	// ErrAddress says the caller's address is outside the client's ranges (403).
	ErrAddress = errors.New("authn: address not allowed for this client")
)

// RateLimited says the client used up its requests of this minute (429).
type RateLimited struct {
	Limit      int
	RetryAfter time.Duration
}

func (e *RateLimited) Error() string {
	return fmt.Sprintf("authn: rate limit of %d requests per minute reached, retry in %s", e.Limit, e.RetryAfter.Round(time.Second))
}

// Client is an API client as the API shows it (never the secret).
type Client struct {
	ClientID   string     `json:"client_id"`
	AccountID  uuid.UUID  `json:"account_id"`
	PersonID   *uuid.UUID `json:"person_id,omitempty"`
	Name       string     `json:"name"`
	IPRanges   []string   `json:"ip_ranges"`
	RateLimit  *int32     `json:"rate_limit,omitempty"`
	TokenTTL   *int32     `json:"token_ttl,omitempty"`
	Enabled    bool       `json:"enabled"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	UpdatedBy  *uuid.UUID `json:"updated_by,omitempty"`
}

// ClientInput is a client as the API takes it; nil keeps the value (or the default).
type ClientInput struct {
	Name      string
	IPRanges  *[]string
	RateLimit *int32
	TokenTTL  *int32
	Enabled   *bool
}

// Token is what the token endpoint answers.
type Token struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// grant is what Valkey holds per token.
type grant struct {
	ClientID    string    `json:"client_id"`
	AccountID   uuid.UUID `json:"account_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func clientOf(r queries.ListClientsRow, person *uuid.UUID) Client {
	c := Client{ClientID: r.ClientID, AccountID: r.AccountID, PersonID: person, Name: r.Name, IPRanges: r.IpRanges, Enabled: r.Enabled,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if c.IPRanges == nil {
		c.IPRanges = []string{}
	}
	if r.RateLimit.Valid {
		v := r.RateLimit.Int32
		c.RateLimit = &v
	}
	if r.TokenTtl.Valid {
		v := r.TokenTtl.Int32
		c.TokenTTL = &v
	}
	if r.LastUsedAt.Valid {
		at := r.LastUsedAt.Time
		c.LastUsedAt = &at
	}
	if r.UpdatedBy.Valid {
		by := r.UpdatedBy.UUID
		c.UpdatedBy = &by
	}
	return c
}

func (s *Service) personOfAccount(ctx context.Context, accountID uuid.UUID) *uuid.UUID {
	acc, err := queries.New(s.pool).GetAccount(ctx, accountID)
	if err != nil || !acc.PersonID.Valid {
		return nil
	}
	p := acc.PersonID.UUID
	return &p
}

// Clients lists the clients.
func (s *Service) Clients(ctx context.Context) ([]Client, error) {
	rows, err := queries.New(s.pool).ListClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("authn: clients: %w", err)
	}
	out := make([]Client, 0, len(rows))
	for _, r := range rows {
		out = append(out, clientOf(r, s.personOfAccount(ctx, r.AccountID)))
	}
	return out, nil
}

// Client returns one client.
func (s *Service) Client(ctx context.Context, id string) (Client, error) {
	r, err := queries.New(s.pool).GetClient(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Client{}, fmt.Errorf("%w: client %s", ErrNotFound, id)
	}
	if err != nil {
		return Client{}, fmt.Errorf("authn: client: %w", err)
	}
	return clientOf(queries.ListClientsRow{ClientID: r.ClientID, AccountID: r.AccountID, Name: r.Name, IpRanges: r.IpRanges, RateLimit: r.RateLimit,
		TokenTtl: r.TokenTtl, Enabled: r.Enabled, LastUsedAt: r.LastUsedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, UpdatedBy: r.UpdatedBy},
		s.personOfAccount(ctx, r.AccountID)), nil
}

func checkRanges(ranges []string) ([]string, error) {
	out := make([]string, 0, len(ranges))
	for _, r := range ranges {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		p, err := netip.ParsePrefix(r)
		if err != nil {
			if a, aerr := netip.ParseAddr(r); aerr == nil {
				p = netip.PrefixFrom(a, a.BitLen())
			} else {
				return nil, fmt.Errorf("%w: ip range %q is no CIDR", ErrInvalid, r)
			}
		}
		out = append(out, p.Masked().String())
	}
	return out, nil
}

func newSecret() string { return random() }

// CreateClient makes account, person and client; the secret is returned this once.
func (s *Service) CreateClient(ctx context.Context, id string, in ClientInput, by *uuid.UUID) (Client, string, error) {
	if in.Name == "" {
		in.Name = id
	}
	ranges := []string{}
	if in.IPRanges != nil {
		var err error
		if ranges, err = checkRanges(*in.IPRanges); err != nil {
			return Client{}, "", err
		}
	}
	q := queries.New(s.pool)
	if taken, err := q.UsernameTaken(ctx, id); err != nil || taken {
		if err != nil {
			return Client{}, "", err
		}
		return Client{}, "", fmt.Errorf("%w: %q is already an account or client", ErrInvalid, id)
	}
	accID, err := uuid.NewV7()
	if err != nil {
		return Client{}, "", err
	}
	acc, err := q.InsertExternalAccount(ctx, queries.InsertExternalAccountParams{ID: accID, Username: id, DisplayName: in.Name})
	if err != nil {
		return Client{}, "", fmt.Errorf("%w: account for client: %w", ErrInvalid, err)
	}
	// A client made in a tenant belongs to it: its account, and its person below (PL45).
	if a := authz.From(ctx); a.Bounded() {
		if err := q.SetAccountTenant(ctx, queries.SetAccountTenantParams{ID: acc.ID, Tenant: pgtype.Text{String: a.CurrentTenant(), Valid: true}}); err != nil {
			return Client{}, "", fmt.Errorf("authn: tenant of the client: %w", err)
		}
	}
	rec, err := s.rec.Create(ctx, "core.person", map[string]any{"last_name": in.Name}, nil)
	if err != nil {
		return Client{}, "", fmt.Errorf("authn: person for client (is packages/core installed?): %w", err)
	}
	pid := uuid.MustParse(rec["id"].(string))
	if err := s.az.LinkPerson(ctx, acc.ID, &pid); err != nil {
		return Client{}, "", err
	}
	secret := newSecret()
	hash, err := auth.HashPassword(secret)
	if err != nil {
		return Client{}, "", err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if err := q.InsertClient(ctx, queries.InsertClientParams{
		ClientID: id, AccountID: acc.ID, Name: in.Name, SecretHash: hash, IpRanges: ranges,
		RateLimit: int4(in.RateLimit), TokenTtl: int4(in.TokenTTL), Enabled: enabled, UpdatedBy: nullUUID(by),
	}); err != nil {
		return Client{}, "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	c, err := s.Client(ctx, id)
	return c, secret, err
}

// UpdateClient changes name, ranges, limits and state.
func (s *Service) UpdateClient(ctx context.Context, id string, in ClientInput, by *uuid.UUID) (Client, error) {
	current, err := s.Client(ctx, id)
	if err != nil {
		return Client{}, err
	}
	name, ranges, enabled := current.Name, current.IPRanges, current.Enabled
	rate, ttl := current.RateLimit, current.TokenTTL
	if in.Name != "" {
		name = in.Name
	}
	if in.IPRanges != nil {
		if ranges, err = checkRanges(*in.IPRanges); err != nil {
			return Client{}, err
		}
	}
	if in.RateLimit != nil {
		rate = in.RateLimit
	}
	if in.TokenTTL != nil {
		ttl = in.TokenTTL
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	n, err := queries.New(s.pool).UpdateClient(ctx, queries.UpdateClientParams{
		ClientID: id, Name: name, IpRanges: ranges, RateLimit: int4(rate), TokenTtl: int4(ttl), Enabled: enabled, UpdatedBy: nullUUID(by),
	})
	if err != nil {
		return Client{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if n == 0 {
		return Client{}, fmt.Errorf("%w: client %s", ErrNotFound, id)
	}
	if !enabled {
		s.revokeTokens(ctx, id)
	}
	return s.Client(ctx, id)
}

// RotateSecret gives the client a new secret; tokens issued so far stay valid.
func (s *Service) RotateSecret(ctx context.Context, id string, by *uuid.UUID) (string, error) {
	secret := newSecret()
	hash, err := auth.HashPassword(secret)
	if err != nil {
		return "", err
	}
	n, err := queries.New(s.pool).SetClientSecret(ctx, queries.SetClientSecretParams{ClientID: id, SecretHash: hash, UpdatedBy: nullUUID(by)})
	if err != nil {
		return "", fmt.Errorf("authn: rotate secret: %w", err)
	}
	if n == 0 {
		return "", fmt.Errorf("%w: client %s", ErrNotFound, id)
	}
	return secret, nil
}

// DeleteClient removes the client and its tokens and blocks its account (the audit keeps the
// account as actor of what the client did).
func (s *Service) DeleteClient(ctx context.Context, id string) error {
	c, err := s.Client(ctx, id)
	if err != nil {
		return err
	}
	if _, err := queries.New(s.pool).DeleteClient(ctx, id); err != nil {
		return fmt.Errorf("authn: delete client: %w", err)
	}
	s.revokeTokens(ctx, id)
	if err := s.auth.SetDisabled(ctx, id, true); err != nil && !errors.Is(err, auth.ErrAccountNotFound) {
		return err
	}
	_ = c
	return nil
}

// revokeTokens drops every token of a client: the tokens are listed under the client.
func (s *Service) revokeTokens(ctx context.Context, id string) {
	if s.kv == nil {
		return
	}
	raw, err := s.kv.Get(ctx, tokenPrefix+"of:"+id)
	if err != nil || len(raw) == 0 {
		return
	}
	for _, t := range strings.Split(string(raw), ",") {
		if t != "" {
			_ = s.kv.Del(ctx, tokenPrefix+t)
		}
	}
	_ = s.kv.Del(ctx, tokenPrefix+"of:"+id)
}

// setting reads an integer setting of the root node, with a default.
func (s *Service) setting(ctx context.Context, key string, def int) int {
	raw, _, err := s.org.Setting(ctx, "root", key)
	if err != nil || len(raw) == 0 {
		return def
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		var str string
		if json.Unmarshal(raw, &str) != nil {
			return def
		}
		if d, err := time.ParseDuration(str); err == nil && key == SettingTokenTTL {
			return int(d.Seconds())
		}
		if v, err := strconv.Atoi(str); err == nil {
			return v
		}
		return def
	}
	if n <= 0 {
		return def
	}
	return int(n)
}

// IssueToken checks client id, secret, state and address and returns a token (RFC 6749 §4.4).
func (s *Service) IssueToken(ctx context.Context, clientID, secret string, addr netip.Addr) (Token, error) {
	q := queries.New(s.pool)
	row, err := q.GetClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_, _ = auth.HashPassword(secret) // the same time as a real check
			return Token{}, ErrInvalidClient
		}
		return Token{}, fmt.Errorf("authn: client: %w", err)
	}
	ok, err := auth.VerifyPassword(row.SecretHash, secret)
	if err != nil {
		return Token{}, err
	}
	if !ok || !row.Enabled {
		s.logger.Info("token refused", "client", clientID, "reason", map[bool]string{true: "disabled", false: "wrong secret"}[row.Enabled && !ok])
		return Token{}, ErrInvalidClient
	}
	if !addressAllowed(row.IpRanges, addr) {
		s.logger.Info("token refused", "client", clientID, "reason", "address", "addr", addr.String())
		return Token{}, ErrAddress
	}
	acc, err := q.GetAccount(ctx, row.AccountID)
	if err != nil || acc.DisabledAt.Valid {
		return Token{}, ErrInvalidClient
	}
	ttl := s.setting(ctx, SettingTokenTTL, defaultTokenTTL)
	if row.TokenTtl.Valid {
		ttl = int(row.TokenTtl.Int32)
	}
	if s.kv == nil {
		return Token{}, errors.New("authn: tokens need Valkey")
	}
	t := random()
	g := grant{ClientID: clientID, AccountID: acc.ID, Username: acc.Username, DisplayName: acc.DisplayName, ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Second)}
	raw, _ := json.Marshal(g)
	if err := s.kv.Set(ctx, tokenPrefix+t, raw, time.Duration(ttl)*time.Second); err != nil {
		return Token{}, err
	}
	// Remember the token under the client, so that disabling the client revokes it.
	list, _ := s.kv.Get(ctx, tokenPrefix+"of:"+clientID)
	_ = s.kv.Set(ctx, tokenPrefix+"of:"+clientID, []byte(strings.Trim(string(list)+","+t, ",")), 30*24*time.Hour)
	_ = q.TouchClient(ctx, clientID)
	s.logger.Info("token issued", "client", clientID, "ttl_seconds", ttl)
	return Token{AccessToken: t, TokenType: "Bearer", ExpiresIn: ttl}, nil
}

// Authenticate turns a bearer token into a session for the request: the token must be live,
// the client enabled, the address in its ranges and the rate not used up (E90).
func (s *Service) Authenticate(ctx context.Context, token string, addr netip.Addr) (auth.Session, error) {
	if s.kv == nil || token == "" {
		return auth.Session{}, ErrInvalidToken
	}
	raw, err := s.kv.Get(ctx, tokenPrefix+token)
	if err != nil || len(raw) == 0 {
		return auth.Session{}, ErrInvalidToken
	}
	var g grant
	if err := json.Unmarshal(raw, &g); err != nil || time.Now().After(g.ExpiresAt) {
		return auth.Session{}, ErrInvalidToken
	}
	row, err := queries.New(s.pool).GetClient(ctx, g.ClientID)
	if err != nil || !row.Enabled {
		return auth.Session{}, ErrInvalidToken
	}
	if !addressAllowed(row.IpRanges, addr) {
		return auth.Session{}, ErrAddress
	}
	limit := s.setting(ctx, SettingRateLimit, defaultRateLimit)
	if row.RateLimit.Valid {
		limit = int(row.RateLimit.Int32)
	}
	now := time.Now()
	window := now.Unix() / 60
	n, err := s.kv.IncrExpire(ctx, fmt.Sprintf("%s%s:%d", ratePrefix, g.ClientID, window), 2*time.Minute)
	if err != nil {
		return auth.Session{}, err
	}
	if n > int64(limit) {
		retry := time.Duration(60-now.Unix()%60) * time.Second
		return auth.Session{}, &RateLimited{Limit: limit, RetryAfter: retry}
	}
	return auth.Session{AccountID: g.AccountID, Username: g.Username, DisplayName: g.DisplayName, CreatedAt: now, RefreshedAt: now}, nil
}

func addressAllowed(ranges []string, addr netip.Addr) bool {
	if len(ranges) == 0 {
		return true
	}
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	for _, r := range ranges {
		p, err := netip.ParsePrefix(r)
		if err != nil {
			continue
		}
		if p.Addr().Is4() && addr.Is4() || p.Addr().Is6() && addr.Is6() {
			if p.Contains(addr) {
				return true
			}
		}
	}
	return false
}

func int4(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}
