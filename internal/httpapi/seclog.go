// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/seclog"
)

// --- security event log (PL27, Hauptplan E102) ---

// secRecord writes an entry with the source of the request: the address (with the proxy rule
// of PL13), the user agent and, when not given, the signed-in account.
func (s *server) secRecord(r *http.Request, e seclog.Event) {
	if s.o.SecLog == nil {
		return
	}
	if sess, ok := auth.SessionFrom(r.Context()); ok {
		if e.Account == nil {
			id := sess.AccountID
			e.Account = &id
		}
		if e.Username == "" {
			e.Username = sess.Username
		}
	}
	e.IP = clientAddr(r, s.o.TrustProxy)
	e.UserAgent = r.UserAgent()
	e.Tenant = s.secTenant(r, e)
	s.o.SecLog.Record(r.Context(), e)
}

// secTenant is the tenant an entry belongs to (PL45): a sign-in that of the account (none for
// an account of the installation or an unknown name); a change that of the session, except a
// change of the installation (roles, permissions, the root).
func (s *server) secTenant(r *http.Request, e seclog.Event) string {
	if e.Tenant != "" {
		return e.Tenant
	}
	if e.Category == seclog.Auth {
		if e.Account == nil || s.o.Authz == nil {
			return ""
		}
		a, err := s.o.Authz.Load(r.Context(), *e.Account)
		if err != nil {
			return ""
		}
		return a.Home
	}
	a := authz.From(r.Context())
	if !a.Bounded() || strings.HasPrefix(e.Kind, "role.") || strings.HasPrefix(e.Kind, "permission.") ||
		e.Target == org.Root || strings.HasPrefix(e.Target, org.Root+" ") {
		return ""
	}
	return a.CurrentTenant()
}

// signIn records a sign-in or another event of the category auth.
func (s *server) signIn(r *http.Request, kind string, account *uuid.UUID, username, reason string, details map[string]any) {
	outcome := seclog.Success
	if reason != "" {
		outcome = seclog.Failure
	}
	s.secRecord(r, seclog.Event{Category: seclog.Auth, Kind: kind, Outcome: outcome, Account: account, Username: username,
		Reason: reason, Details: details})
}

// changed records a successful change of accounts, rights, rules, clients, directories or
// settings with the state before and after (either may be nil).
func (s *server) changed(r *http.Request, kind, target string, before, after any, err error) {
	if err != nil {
		return
	}
	details := map[string]any{}
	if before != nil {
		details["before"] = before
	}
	if after != nil {
		details["after"] = after
	}
	s.secRecord(r, seclog.Event{Category: seclog.Change, Kind: kind, Outcome: seclog.Success, Target: target, Details: details})
}

func (s *server) secReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.SecLog == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "the security event log is not configured on this instance", nil)
		return false
	}
	if a := authz.From(r.Context()); !a.Allowed(seclog.Type, "read") {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "the security event log needs the right read on "+seclog.Type, nil)
		return false
	}
	return true
}

func secFilter(category string, kind, outcome, username, ip *string, account *uuid.UUID, since, until *time.Time, before *int64, limit *int) (seclog.Filter, string) {
	f := seclog.Filter{Category: category, Account: account}
	if kind != nil {
		f.Kind = *kind
	}
	if outcome != nil {
		f.Outcome = *outcome
	}
	if username != nil {
		f.Username = *username
	}
	if ip != nil && *ip != "" {
		a, err := netip.ParseAddr(*ip)
		if err != nil {
			return f, "ip must be an IPv4 or IPv6 address"
		}
		f.IP = a.Unmap()
	}
	if since != nil {
		f.Since = *since
	}
	if until != nil {
		f.Until = *until
	}
	if before != nil {
		f.Before = *before
	}
	if limit != nil {
		f.Limit = *limit
	}
	return f, ""
}

func (s *server) listSecurity(w http.ResponseWriter, r *http.Request, f seclog.Filter, bad string) {
	if !s.secReady(w, r) {
		return
	}
	if a := authz.From(r.Context()); a.Bounded() {
		f.Tenant, f.Installation = a.CurrentTenant(), a.Installation(seclog.Type, "read")
	}
	if bad != "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", bad, nil)
		return
	}
	page, err := s.o.SecLog.List(r.Context(), f)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *server) AuditAuth(w http.ResponseWriter, r *http.Request, p gen.AuditAuthParams) {
	f, bad := secFilter(seclog.Auth, p.Kind, p.Outcome, p.Username, p.Ip, p.Account, p.Since, p.Until, p.Before, p.Limit)
	s.listSecurity(w, r, f, bad)
}

func (s *server) AuditChanges(w http.ResponseWriter, r *http.Request, p gen.AuditChangesParams) {
	f, bad := secFilter(seclog.Change, p.Kind, p.Outcome, p.Username, p.Ip, p.Account, p.Since, p.Until, p.Before, p.Limit)
	s.listSecurity(w, r, f, bad)
}

// AuditData lists the exports (UI5).
func (s *server) AuditData(w http.ResponseWriter, r *http.Request, p gen.AuditDataParams) {
	f, bad := secFilter(seclog.Data, p.Kind, p.Outcome, p.Username, p.Ip, p.Account, p.Since, p.Until, p.Before, p.Limit)
	s.listSecurity(w, r, f, bad)
}
