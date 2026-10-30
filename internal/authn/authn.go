// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package authn signs people in through an identity provider (Hauptplan E10, E61, E88;
// PL11): OpenID Connect with the authorization code flow, PKCE and a nonce. The first
// sign-in of a subject creates an account without a local password and a person; login
// rules (G121, rules.go) assign roles from the claims at every sign-in. Local accounts and
// sessions stay in internal/auth; LDAP (PL12) and API clients (PL13) join here later.
package authn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
)

// CallbackPath is where the provider sends the browser back to; the public URL completes it.
const CallbackPath = "/api/v1/auth/oidc/callback"

// pendingTTL is how long a started sign-in waits for the provider.
const pendingTTL = 10 * time.Minute

var (
	// ErrDisabled says no provider is configured.
	ErrDisabled = errors.New("authn: no identity provider configured")
	// ErrState says the callback does not belong to a sign-in started here (or it expired).
	ErrState = errors.New("authn: unknown or expired sign-in state")
	// ErrRefused says the provider signed the person in but the account may not be used.
	ErrRefused = errors.New("authn: account disabled")
	// ErrInvalid says a rule cannot be stored as given.
	ErrInvalid = errors.New("authn: invalid")
	// ErrNotFound says there is no such rule.
	ErrNotFound = errors.New("authn: not found")
)

// Options configure the one provider (E88); Issuer empty means sign-in through a provider
// is off.
type Options struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	// Name is the label of the button on the sign-in page ("SSO" when empty).
	Name string
	// PublicURL is this server as the browser reaches it, for the redirect back.
	PublicURL string
	// InternalURL, when set, is the provider as this process reaches it (Compose: the
	// browser sees localhost:8091, the container keycloak:8080); the issuer stays as is.
	InternalURL string
	Scopes      []string
	// GroupsClaim names the claim with the groups ("groups" when empty); rules see every
	// claim anyway, this one is only logged.
	GroupsClaim string
}

// Service runs the sign-in and the login rules.
type Service struct {
	o      Options
	pool   *pgxpool.Pool
	kv     *cache.Client
	auth   *auth.Service
	az     *authz.Service
	rec    *records.Service
	org    *org.Service
	logger *slog.Logger
	client *http.Client

	mu       sync.Mutex
	provider *oidc.Provider
	oauth    *oauth2.Config
}

// New wires the service without touching the network; discovery runs at first use and is
// retried, so a provider that is still starting does not stop the server.
func New(pool *pgxpool.Pool, kv *cache.Client, a *auth.Service, az *authz.Service, rec *records.Service, o *org.Service, logger *slog.Logger, opts Options) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if opts.Name == "" {
		opts.Name = "SSO"
	}
	if len(opts.Scopes) == 0 {
		opts.Scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	if opts.GroupsClaim == "" {
		opts.GroupsClaim = "groups"
	}
	s := &Service{o: opts, pool: pool, kv: kv, auth: a, az: az, rec: rec, org: o, logger: logger}
	s.client = &http.Client{Timeout: 15 * time.Second, Transport: http.DefaultTransport}
	if opts.InternalURL != "" {
		if from, err := url.Parse(opts.Issuer); err == nil {
			if to, err := url.Parse(opts.InternalURL); err == nil {
				s.client.Transport = &rewrite{from: from, to: to, next: http.DefaultTransport}
			}
		}
	}
	return s
}

// Enabled says whether a provider is configured.
func (s *Service) Enabled() bool { return s != nil && s.o.Issuer != "" && s.o.ClientID != "" }

// Name is the label of the provider for the sign-in page.
func (s *Service) Name() string { return s.o.Name }

// SetPublicURL sets the base URL of this server (tests learn it after the server started).
func (s *Service) SetPublicURL(u string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.o.PublicURL = u
	if s.oauth != nil {
		s.oauth.RedirectURL = strings.TrimRight(u, "/") + CallbackPath
	}
}

// rewrite sends requests meant for the public issuer host to the internal host.
type rewrite struct {
	from, to *url.URL
	next     http.RoundTripper
}

func (t *rewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == t.from.Host && r.URL.Scheme == t.from.Scheme {
		r2 := r.Clone(r.Context())
		r2.URL.Scheme, r2.URL.Host, r2.Host = t.to.Scheme, t.to.Host, t.to.Host
		return t.next.RoundTrip(r2)
	}
	return t.next.RoundTrip(r)
}

func (s *Service) withClient(ctx context.Context) context.Context {
	return context.WithValue(oidc.ClientContext(ctx, s.client), oauth2.HTTPClient, s.client)
}

// ready runs the discovery once and keeps the result.
func (s *Service) ready(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	if !s.Enabled() {
		return nil, nil, ErrDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provider != nil {
		return s.provider, s.oauth, nil
	}
	p, err := oidc.NewProvider(s.withClient(ctx), s.o.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("authn: discovery of %s: %w", s.o.Issuer, err)
	}
	s.provider = p
	s.oauth = &oauth2.Config{
		ClientID: s.o.ClientID, ClientSecret: s.o.ClientSecret, Endpoint: p.Endpoint(),
		RedirectURL: strings.TrimRight(s.o.PublicURL, "/") + CallbackPath, Scopes: s.o.Scopes,
	}
	s.logger.Info("identity provider ready", "issuer", s.o.Issuer, "name", s.o.Name)
	return s.provider, s.oauth, nil
}

// pending is a started sign-in, kept in Valkey under its state until the callback.
type pending struct {
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Next     string `json:"next"`
}

func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Begin starts a sign-in and returns the URL of the provider to send the browser to. next
// is the path to continue at afterwards (only absolute paths of this site are kept).
func (s *Service) Begin(ctx context.Context, next string) (string, error) {
	_, cfg, err := s.ready(ctx)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}
	state, p := random(), pending{Nonce: random(), Verifier: oauth2.GenerateVerifier(), Next: next}
	raw, _ := json.Marshal(p)
	if err := s.kv.Set(ctx, "authn:state:"+state, raw, pendingTTL); err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(state, oidc.Nonce(p.Nonce), oauth2.S256ChallengeOption(p.Verifier)), nil
}

// Claims is what the provider said about the person, as the rules see it.
type Claims map[string]any

func (c Claims) str(key string) string {
	v, _ := c[key].(string)
	return v
}

// Complete finishes a sign-in with the callback's state and code: the code is exchanged,
// the ID token verified (signature, audience, expiry, nonce), the account found or created,
// the rules applied, and a session created. It returns the session and where to continue.
func (s *Service) Complete(ctx context.Context, state, code string) (auth.Session, string, error) {
	provider, cfg, err := s.ready(ctx)
	if err != nil {
		return auth.Session{}, "", err
	}
	raw, err := s.kv.Get(ctx, "authn:state:"+state)
	if err != nil || len(raw) == 0 || state == "" {
		return auth.Session{}, "", ErrState
	}
	_ = s.kv.Del(ctx, "authn:state:"+state)
	var p pending
	if err := json.Unmarshal(raw, &p); err != nil {
		return auth.Session{}, "", ErrState
	}
	token, err := cfg.Exchange(s.withClient(ctx), code, oauth2.VerifierOption(p.Verifier))
	if err != nil {
		return auth.Session{}, "", fmt.Errorf("authn: code exchange: %w", err)
	}
	rawID, _ := token.Extra("id_token").(string)
	if rawID == "" {
		return auth.Session{}, "", errors.New("authn: the provider sent no ID token")
	}
	idt, err := provider.Verifier(&oidc.Config{ClientID: s.o.ClientID}).Verify(s.withClient(ctx), rawID)
	if err != nil {
		return auth.Session{}, "", fmt.Errorf("authn: ID token: %w", err)
	}
	if idt.Nonce != p.Nonce {
		return auth.Session{}, "", errors.New("authn: ID token nonce does not match")
	}
	claims := Claims{}
	if err := idt.Claims(&claims); err != nil {
		return auth.Session{}, "", fmt.Errorf("authn: claims: %w", err)
	}
	acc, person, err := s.account(ctx, idt.Issuer, idt.Subject, claims)
	if err != nil {
		return auth.Session{}, "", err
	}
	if err := s.ApplyRules(ctx, person, acc.Tenant.String, claims); err != nil {
		return auth.Session{}, "", err
	}
	sess, err := s.auth.Sessions().Create(ctx, auth.Session{AccountID: acc.ID, Username: acc.Username, DisplayName: acc.DisplayName})
	if err != nil {
		return auth.Session{}, "", err
	}
	s.logger.Info("login", "username", acc.Username, "provider", s.o.Name, "groups", claims[s.o.GroupsClaim])
	return sess, p.Next, nil
}

var usernameChars = regexp.MustCompile(`[^a-z0-9_]+`)

// usernameFrom derives a snake_case username (E75) from the claims.
func usernameFrom(claims Claims) string {
	base := claims.str("preferred_username")
	if base == "" {
		base = claims.str("email")
	}
	if i := strings.IndexByte(base, '@'); i > 0 {
		base = base[:i]
	}
	base = usernameChars.ReplaceAllString(strings.ToLower(base), "_")
	base = strings.Trim(base, "_")
	if base == "" || base[0] < 'a' || base[0] > 'z' {
		base = "u_" + base
	}
	if len(base) > 60 {
		base = base[:60]
	}
	return strings.TrimRight(base, "_")
}

// account finds the account of the identity or creates account, person and identity.
func (s *Service) account(ctx context.Context, issuer, subject string, claims Claims) (queries.Account, uuid.UUID, error) {
	q := queries.New(s.pool)
	email := claims.str("email")
	var emailArg pgtype.Text
	if email != "" {
		emailArg = pgtype.Text{String: email, Valid: true}
	}
	rawClaims, _ := json.Marshal(claims)
	ident, err := q.GetIdentity(ctx, queries.GetIdentityParams{Issuer: issuer, Subject: subject})
	switch {
	case err == nil:
		acc, err := q.GetAccount(ctx, ident.AccountID)
		if err != nil {
			return queries.Account{}, uuid.Nil, fmt.Errorf("authn: account of identity: %w", err)
		}
		if acc.DisabledAt.Valid {
			s.logger.Info("login refused", "username", acc.Username, "reason", "account disabled", "provider", s.o.Name)
			return queries.Account{}, uuid.Nil, ErrRefused
		}
		if err := q.UpsertIdentity(ctx, queries.UpsertIdentityParams{Issuer: issuer, Subject: subject, AccountID: acc.ID, Email: emailArg, Claims: rawClaims}); err != nil {
			return queries.Account{}, uuid.Nil, fmt.Errorf("authn: identity: %w", err)
		}
		person, err := s.personOf(ctx, acc, claims)
		return acc, person, err
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return queries.Account{}, uuid.Nil, fmt.Errorf("authn: identity: %w", err)
	}

	// First sign-in: account without a local password, person, identity (E88).
	username := usernameFrom(claims)
	for i := 2; ; i++ {
		taken, err := q.UsernameTaken(ctx, username)
		if err != nil {
			return queries.Account{}, uuid.Nil, err
		}
		if !taken {
			break
		}
		username = fmt.Sprintf("%s_%d", usernameFrom(claims), i)
	}
	display := claims.str("name")
	if display == "" {
		display = username
	}
	id, err := uuid.NewV7()
	if err != nil {
		return queries.Account{}, uuid.Nil, err
	}
	acc, err := q.InsertExternalAccount(ctx, queries.InsertExternalAccountParams{ID: id, Username: username, DisplayName: display})
	if err != nil {
		return queries.Account{}, uuid.Nil, fmt.Errorf("authn: create account: %w", err)
	}
	if err := q.UpsertIdentity(ctx, queries.UpsertIdentityParams{Issuer: issuer, Subject: subject, AccountID: acc.ID, Email: emailArg, Claims: rawClaims}); err != nil {
		return queries.Account{}, uuid.Nil, fmt.Errorf("authn: identity: %w", err)
	}
	s.logger.Info("account created from identity provider", "username", username, "provider", s.o.Name)
	// With tenants the account belongs to the tenant of the first login rule that matches; with
	// none it is an account of the installation that reaches no tenant (PL45).
	if tenant, err := s.tenantByRules(ctx, claims); err != nil {
		return queries.Account{}, uuid.Nil, err
	} else if tenant != "" {
		acc.Tenant = pgtype.Text{String: tenant, Valid: true}
		if err := q.SetAccountTenant(ctx, queries.SetAccountTenantParams{ID: acc.ID, Tenant: acc.Tenant}); err != nil {
			return queries.Account{}, uuid.Nil, fmt.Errorf("authn: tenant of the account: %w", err)
		}
	}
	person, err := s.personOf(ctx, acc, claims)
	return acc, person, err
}

// personOf returns the person of the account; without one, a person with the same email
// (from a directory sync, E89) is linked, else one is created from the claims. Both in the
// tenant of the account (PL45).
func (s *Service) personOf(ctx context.Context, acc queries.Account, claims Claims) (uuid.UUID, error) {
	if acc.PersonID.Valid {
		return acc.PersonID.UUID, nil
	}
	ctx = authz.WithAccess(ctx, authz.SystemIn(acc.Tenant.String))
	if pid := s.personByEmail(ctx, claims.str("email")); pid != nil {
		if err := s.az.LinkPerson(ctx, acc.ID, pid); err != nil {
			return uuid.Nil, err
		}
		s.logger.Info("account linked to existing person", "username", acc.Username, "person", pid.String())
		return *pid, nil
	}
	last := claims.str("family_name")
	if last == "" {
		last = acc.DisplayName
	}
	values := map[string]any{"last_name": last}
	if first := claims.str("given_name"); first != "" {
		values["first_name"] = first
	}
	if email := claims.str("email"); email != "" {
		values["email"] = email
	}
	rec, err := s.rec.Create(ctx, "core.person", values, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("authn: create person (is packages/core installed?): %w", err)
	}
	pid := uuid.MustParse(rec["id"].(string))
	if err := s.az.LinkPerson(ctx, acc.ID, &pid); err != nil {
		return uuid.Nil, err
	}
	return pid, nil
}
