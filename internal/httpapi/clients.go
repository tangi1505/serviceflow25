// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
)

// --- API clients and the token endpoint (PL13) ---

// tokenPath is the OAuth 2 token endpoint; public and without the CSRF header, because
// machines call it with a form body and no cookie is involved (E90).
const tokenPath = "/api/v1/auth/token" //nolint:gosec // G101: a path, no credential

// clientAddr is the caller's address: the peer, or the first X-Forwarded-For entry when the
// server sits behind a trusted proxy (ITSM_TRUST_PROXY).
func clientAddr(r *http.Request, trustProxy bool) netip.Addr {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if a, err := netip.ParseAddr(first); err == nil {
				return a.Unmap()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

// IssueToken is POST /api/v1/auth/token: grant_type=client_credentials with the client in
// the form or in HTTP Basic (RFC 6749 §4.4); errors come as RFC 6749 §5.2 answers.
func (s *server) IssueToken(w http.ResponseWriter, r *http.Request) {
	oauthError := func(status int, code, detail string) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, status, map[string]string{"error": code, "error_description": detail})
	}
	if s.o.Authn == nil {
		oauthError(http.StatusServiceUnavailable, "temporarily_unavailable", "API clients are not configured on this instance")
		return
	}
	if err := r.ParseForm(); err != nil {
		oauthError(http.StatusBadRequest, "invalid_request", "the body must be application/x-www-form-urlencoded")
		return
	}
	if r.PostForm.Get("grant_type") != "client_credentials" {
		oauthError(http.StatusBadRequest, "unsupported_grant_type", "only client_credentials is supported")
		return
	}
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	if id == "" || secret == "" {
		oauthError(http.StatusBadRequest, "invalid_request", "client_id and client_secret are required")
		return
	}
	token, err := s.o.Authn.IssueToken(r.Context(), id, secret, clientAddr(r, s.o.TrustProxy))
	var lock *auth.LockError
	switch {
	case errors.As(err, &lock):
		// The client or the address is locked after refused requests (AP6).
		s.lockedAttempt(r, "token", id, lock)
		retryAfter(w, lock.RetryAfter)
		oauthError(http.StatusTooManyRequests, "slow_down", "too many refused requests; try again later")
		return
	case errors.Is(err, auth.ErrBusy):
		retryAfter(w, time.Second)
		oauthError(http.StatusServiceUnavailable, "temporarily_unavailable", "too many sign-ins at once; try again in a second")
		return
	case errors.Is(err, authn.ErrInvalidClient):
		s.signIn(r, "token", nil, id, "invalid_client", nil)
		w.Header().Set("WWW-Authenticate", `Basic realm="itsm"`)
		oauthError(http.StatusUnauthorized, "invalid_client", "unknown client, wrong secret or client disabled")
		return
	case errors.Is(err, authn.ErrAddress):
		s.signIn(r, "token", nil, id, "address_not_allowed", nil)
		oauthError(http.StatusForbidden, "access_denied", "this address is not allowed for the client")
		return
	case err != nil:
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.signIn(r, "token", nil, id, "", nil)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, token)
}

// bearer authenticates a request with `Authorization: Bearer`; it answers the request itself
// when the token is refused and reports whether the caller may go on.
func (s *server) bearer(w http.ResponseWriter, r *http.Request, token string) (sess sessionOf, ok bool) {
	if s.o.Authn == nil {
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "bearer tokens are not accepted on this instance", nil)
		return sessionOf{}, false
	}
	session, err := s.o.Authn.Authenticate(r.Context(), token, clientAddr(r, s.o.TrustProxy))
	var limited *authn.RateLimited
	switch {
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.Itoa(int(limited.RetryAfter.Seconds())+1))
		writeProblem(w, r, http.StatusTooManyRequests, "rate_limited", err.Error(), nil)
		return sessionOf{}, false
	case errors.Is(err, authn.ErrAddress):
		writeProblem(w, r, http.StatusForbidden, "forbidden", "this address is not allowed for the client", nil)
		return sessionOf{}, false
	case errors.Is(err, authn.ErrInvalidToken):
		w.Header().Set("WWW-Authenticate", `Bearer realm="itsm"`)
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "unknown or expired token", nil)
		return sessionOf{}, false
	case err != nil:
		writeError(w, r, s.o.Logger, err)
		return sessionOf{}, false
	}
	return sessionOf{session}, true
}

// --- management of clients (manage_rights) ---

func (s *server) clientsReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Authn == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "API clients are not configured on this instance", nil)
		return false
	}
	return s.mayManageRights(w, r)
}

func clientInput(body gen.ClientInput) authn.ClientInput {
	in := authn.ClientInput{IPRanges: body.IpRanges, Enabled: body.Enabled}
	if body.Name != nil {
		in.Name = *body.Name
	}
	if body.RateLimit != nil {
		v := int32(*body.RateLimit) //nolint:gosec // the schema bounds it
		in.RateLimit = &v
	}
	if body.TokenTtl != nil {
		v := int32(*body.TokenTtl) //nolint:gosec // the schema bounds it
		in.TokenTTL = &v
	}
	return in
}

func (s *server) ListClients(w http.ResponseWriter, r *http.Request) {
	if !s.clientsReady(w, r) {
		return
	}
	list, err := s.o.Authn.Clients(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	mine := []authn.Client{}
	for _, c := range list {
		if s.ownClient(r, c) {
			mine = append(mine, c)
		}
	}
	writeJSON(w, http.StatusOK, mine)
}

// ownClient reports whether a client belongs to the session's tenant: its account does; a
// client of the installation needs manage_rights at the root (PL45).
func (s *server) ownClient(r *http.Request, c authn.Client) bool {
	a := authz.From(r.Context())
	if !a.Bounded() || s.o.Authz == nil {
		return true
	}
	acc, err := s.o.Authz.Load(r.Context(), c.AccountID)
	if err != nil {
		return false
	}
	if acc.Home == "" {
		return a.Installation(rightsType, "manage_rights")
	}
	return acc.Home == a.CurrentTenant()
}

// clientInTenant answers 404 for a client of another tenant.
func (s *server) clientInTenant(w http.ResponseWriter, r *http.Request, id string) bool {
	c, err := s.o.Authn.Client(r.Context(), id)
	if err == nil && !s.ownClient(r, c) {
		err = fmt.Errorf("%w: client %s", authn.ErrNotFound, id)
	}
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return false
	}
	return true
}

// clientWithSecret is the answer of create and rotate: the secret shows this once.
type clientWithSecret struct {
	authn.Client
	Secret string `json:"secret"`
}

func (s *server) CreateClient(w http.ResponseWriter, r *http.Request) {
	if !s.clientsReady(w, r) {
		return
	}
	var body gen.ClientCreate
	if !decode(w, r, &body) {
		return
	}
	in := clientInput(gen.ClientInput{Name: body.Name, IpRanges: body.IpRanges, RateLimit: body.RateLimit, TokenTtl: body.TokenTtl, Enabled: body.Enabled})
	c, secret, err := s.o.Authn.CreateClient(r.Context(), body.ClientId, in, actor(r))
	s.changed(r, "api_client.create", body.ClientId, nil, c, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/clients/"+c.ClientID)
	writeJSON(w, http.StatusCreated, clientWithSecret{c, secret})
}

func (s *server) GetClient(w http.ResponseWriter, r *http.Request, id gen.ClientId) {
	if !s.clientsReady(w, r) || !s.clientInTenant(w, r, id) {
		return
	}
	c, err := s.o.Authn.Client(r.Context(), id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) UpdateClient(w http.ResponseWriter, r *http.Request, id gen.ClientId) {
	if !s.clientsReady(w, r) {
		return
	}
	var body gen.ClientInput
	if !decode(w, r, &body) || !s.clientInTenant(w, r, id) {
		return
	}
	before, _ := s.o.Authn.Client(r.Context(), id)
	c, err := s.o.Authn.UpdateClient(r.Context(), id, clientInput(body), actor(r))
	s.changed(r, "api_client.update", id, before, c, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) RotateClientSecret(w http.ResponseWriter, r *http.Request, id gen.ClientId) {
	if !s.clientsReady(w, r) || !s.clientInTenant(w, r, id) {
		return
	}
	secret, err := s.o.Authn.RotateSecret(r.Context(), id, actor(r))
	s.changed(r, "api_client.rotate", id, nil, nil, err)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	c, err := s.o.Authn.Client(r.Context(), id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, clientWithSecret{c, secret})
}

func (s *server) DeleteClient(w http.ResponseWriter, r *http.Request, id gen.ClientId) {
	if !s.clientsReady(w, r) || !s.clientInTenant(w, r, id) {
		return
	}
	before, _ := s.o.Authn.Client(r.Context(), id)
	err := s.o.Authn.DeleteClient(r.Context(), id)
	s.changed(r, "api_client.delete", id, before, nil, err)
	s.noContent(w, r, err)
}
