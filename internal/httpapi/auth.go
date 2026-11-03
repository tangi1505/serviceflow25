// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seclog"
)

// Session cookie and CSRF header (Hauptplan E74): the cookie is HttpOnly, SameSite=Lax and
// Secure behind TLS; every unsafe request under /api must carry X-Requested-With: itsm, which a
// cross-site form cannot add.
const (
	sessionCookie = "itsm_session"
	csrfHeader    = "X-Requested-With"
	csrfValue     = "itsm"
)

// public lists the paths under /api that need no session: the version for the sign-in page of
// the web interface, and signing in itself (E78).
var public = map[string]bool{
	"/api/v1/version":            true,
	"/api/v1/auth/login":         true,
	tokenPath:                    true,
	"/api/v1/auth/providers":     true,
	"/api/v1/auth/oidc/start":    true,
	"/api/v1/auth/oidc/callback": true,
}

// whileExpired lists the paths that stay allowed while the password is expired (E77).
var whileExpired = map[string]bool{
	"/api/v1/auth/me":       true,
	"/api/v1/auth/password": true,
	"/api/v1/auth/logout":   true,
}

// authenticate guards /api: CSRF header on unsafe methods, then the session cookie, then the
// password-expired block. Health endpoints and the web interface pass untouched.
func authenticate(o Options) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if !strings.HasPrefix(path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			// A bearer token (API clients, PL13) carries no cookie, so CSRF does not apply.
			bearerToken := ""
			if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
				bearerToken = strings.TrimSpace(h[7:])
			}
			if bearerToken == "" && path != tokenPath {
				switch r.Method {
				case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
					if r.Header.Get(csrfHeader) != csrfValue {
						writeProblem(w, r, http.StatusForbidden, "forbidden", "header "+csrfHeader+": "+csrfValue+" required", nil)
						return
					}
				}
			}
			if public[path] {
				next.ServeHTTP(w, r)
				return
			}
			if o.Auth == nil {
				writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "sign-in is not available", nil)
				return
			}
			var sess auth.Session
			var err error
			if bearerToken != "" {
				s, ok := (&server{o: o}).bearer(w, r, bearerToken)
				if !ok {
					return
				}
				sess = s.Session
			} else {
				id := ""
				if c, err := r.Cookie(sessionCookie); err == nil {
					id = c.Value
				}
				sess, err = o.Auth.Sessions().Get(r.Context(), id)
				if err != nil {
					if errors.Is(err, auth.ErrNoSession) {
						clearSessionCookie(w, r)
						writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "sign in first", nil)
						return
					}
					writeError(w, r, o.Logger, err)
					return
				}
			}
			if sess.PasswordExpired && !whileExpired[path] {
				writeProblem(w, r, http.StatusForbidden, "password_expired", "change the password first", nil)
				return
			}
			// The rights of the session (PL8): loaded once per request, applied by records.
			access := &authz.Access{AccountID: sess.AccountID}
			if o.Authz != nil {
				access, err = o.Authz.Load(r.Context(), sess.AccountID)
				if err != nil {
					writeError(w, r, o.Logger, err)
					return
				}
			}
			// The tenant of the session (PL44): a copy of the cached access works in it. An
			// account of the installation starts in the tenant it chose last, else the first.
			if sess.Tenant == "" && len(access.Tenants) > 1 && bearerToken == "" {
				last := ""
				if o.Prefs != nil {
					last = o.Prefs.LastTenant(r.Context(), sess.AccountID)
				}
				sess.Tenant = access.Enter(last).Tenant
				if err := o.Auth.Sessions().Update(r.Context(), sess); err != nil {
					writeError(w, r, o.Logger, err)
					return
				}
			}
			access = access.Enter(sess.Tenant)
			// An administrator of the installation entering a tenant with a database of its own
			// brings the mirror there up to date: after signing in and after every switch (PL47b).
			if o.Authn != nil && o.Authn.Mirrors() && access.Home == "" && bearerToken == "" && sess.Entered != access.CurrentTenant() {
				if err := o.Authn.Mirror(r.Context(), sess.AccountID, access.CurrentTenant()); err != nil {
					writeError(w, r, o.Logger, err)
					return
				}
				sess.Entered = access.CurrentTenant()
				if err := o.Auth.Sessions().Update(r.Context(), sess); err != nil {
					writeError(w, r, o.Logger, err)
					return
				}
			}
			// The session narrows it further (PL10).
			access.ActiveRole, access.ActiveOrg = sess.ActiveRole, sess.ActiveOrg
			ctx := auth.WithSession(r.Context(), sess)
			next.ServeHTTP(w, r.WithContext(authz.WithAccess(ctx, access)))
		})
	}
}

// sessionOf wraps the session a bearer token stands for.
type sessionOf struct{ auth.Session }

// actor is who performs a write: the account of the session.
func actor(r *http.Request) records.Actor {
	if s, ok := auth.SessionFrom(r.Context()); ok {
		id := s.AccountID
		return &id
	}
	return nil
}

func secure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, s auth.Session) {
	// Secure follows TLS (E74): development runs on plain HTTP behind no proxy.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is set behind TLS, see above
		Name: sessionCookie, Value: s.ID, Path: "/", HttpOnly: true, Secure: secure(r),
		SameSite: http.SameSiteLaxMode, Expires: s.CreatedAt.Add(auth.MaxLifetime),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is set behind TLS, see setSessionCookie
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: secure(r),
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// describe is me plus the tenants (PL44): the one the session works in and those it may
// switch to, with their labels.
func (s *server) describe(r *http.Request, sess auth.Session, a *authz.Access) gen.Me {
	m := me(sess, a)
	if a == nil || a.IsSystem() {
		return m
	}
	t := a.CurrentTenant()
	m.Tenant = &t
	nodes, err := s.o.Org.Tenants(r.Context())
	if err != nil || len(nodes) == 0 {
		return m
	}
	refs := []gen.TenantRef{}
	for _, n := range nodes {
		if contains(a.Tenants, n.Path) {
			refs = append(refs, gen.TenantRef{Path: n.Path, Label: n.Label})
		}
	}
	m.Tenants = &refs
	return m
}

func me(s auth.Session, a *authz.Access) gen.Me {
	m := gen.Me{Id: s.AccountID, Username: s.Username, DisplayName: s.DisplayName,
		PasswordExpired: s.PasswordExpired, SessionExpiresAt: s.ExpiresAt()}
	if s.ActiveRole != "" {
		m.ActiveRole = &s.ActiveRole
	}
	if s.ActiveOrg != "" {
		m.ActiveOrg = &s.ActiveOrg
	}
	// The workplace for agents, the portal for everyone else (UI10).
	workplace := a == nil || a.IsSystem() || a.Allowed("core.ui", "workplace")
	m.Workplace = &workplace
	// Changing types in the customer layer (PL43): the interface shows its editor (UI25).
	customize := a != nil && !a.IsSystem() && a.Installation(typeRights, "customize")
	m.Customize = &customize
	// Managing roles, their permissions and assignments (PL8): the interface offers the role
	// editor (UI27).
	manageRights := a != nil && !a.IsSystem() && a.Allowed(rightsType, "manage_rights")
	m.ManageRights = &manageRights
	if a != nil && a.PersonID != nil {
		m.PersonId = a.PersonID
	}
	if a != nil && !a.IsSystem() {
		roles, nodes := a.Roles(), a.Nodes()
		if roles == nil {
			roles = []string{}
		}
		if nodes == nil {
			nodes = []string{}
		}
		m.Roles, m.Nodes = &roles, &nodes
	}
	return m
}

// SetContext narrows the session to a role and a node the person holds (PL10), and switches
// the tenant of an account of the installation (PL44): role and node then start afresh.
func (s *server) SetContext(w http.ResponseWriter, r *http.Request) {
	var body gen.SessionContext
	if !decode(w, r, &body) {
		return
	}
	sess, _ := auth.SessionFrom(r.Context())
	// The whole access, not the narrowed one, decides what may be chosen.
	whole, err := s.o.Authz.Load(r.Context(), sess.AccountID)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	before := authz.From(r.Context()).CurrentTenant()
	tenant := sess.Tenant
	if body.Tenant != nil {
		tenant = *body.Tenant
		if !contains(whole.Tenants, tenant) || tenant == authz.NoTenant {
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{
				{Field: "tenant", Code: "unknown", Message: "you may not work in this tenant"}})
			return
		}
	}
	full := whole.Enter(tenant)
	role, node := "", ""
	if body.Role != nil {
		role = *body.Role
	}
	if body.OrgPath != nil {
		node = *body.OrgPath
	}
	if role != "" && !contains(full.Roles(), role) {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{
			{Field: "role", Code: "unknown", Message: "you do not hold this role"}})
		return
	}
	if node != "" {
		ok := false
		for _, n := range full.Nodes() {
			if (node == n || strings.HasPrefix(node, n+".")) && full.InTenant(node) {
				ok = true
			}
		}
		if exists, err := s.o.Org.Exists(r.Context(), node); err != nil {
			writeError(w, r, s.o.Logger, err)
			return
		} else if !ok || !exists {
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{
				{Field: "org_path", Code: "unknown", Message: "you hold no role at or above this node"}})
			return
		}
	}
	sess.ActiveRole, sess.ActiveOrg, sess.Tenant = role, node, full.Tenant
	if err := s.o.Auth.Sessions().Update(r.Context(), sess); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	full.ActiveRole, full.ActiveOrg = role, node
	if after := full.CurrentTenant(); after != before {
		// A switch of the tenant stands in the security log (V27) and is kept for the next
		// session.
		s.signIn(r, "tenant", nil, "", "", map[string]any{"from": before, "to": after})
		if s.o.Prefs != nil {
			if err := s.o.Prefs.RememberTenant(r.Context(), sess.AccountID, after); err != nil {
				s.o.Logger.Warn("auth: remember the tenant", "error", err.Error())
			}
		}
	}
	s.signIn(r, "context", nil, "", "", map[string]any{"role": role, "org_path": node, "tenant": full.CurrentTenant()})
	writeJSON(w, http.StatusOK, s.describe(r, sess, full))
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "body: "+err.Error(), nil)
		return false
	}
	return true
}

// --- auth ---

func (s *server) Login(w http.ResponseWriter, r *http.Request) {
	var c gen.Credentials
	if !decode(w, r, &c) {
		return
	}
	if c.Username == "" || c.Password == "" {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{
			{Field: "username", Code: "required", Message: "username and password are required"}})
		return
	}
	sess, err := s.o.Auth.Login(r.Context(), c.Username, c.Password, clientAddr(r, s.o.TrustProxy))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			reason, account := "invalid_credentials", (*uuid.UUID)(nil)
			var le *auth.LoginError
			if errors.As(err, &le) {
				reason, account = le.Reason, le.Account
			}
			s.signIn(r, "login", account, c.Username, reason, nil)
			writeProblem(w, r, http.StatusUnauthorized, "invalid_credentials", "unknown account or wrong password", nil)
			return
		}
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.signIn(r, "login", &sess.AccountID, sess.Username, "", nil)
	setSessionCookie(w, r, sess)
	var access *authz.Access
	if s.o.Authz != nil {
		if a, err := s.o.Authz.Load(r.Context(), sess.AccountID); err == nil {
			last := ""
			if s.o.Prefs != nil && len(a.Tenants) > 1 {
				last = s.o.Prefs.LastTenant(r.Context(), sess.AccountID)
			}
			access = a.Enter(last)
		}
	}
	writeJSON(w, http.StatusOK, s.describe(r, sess, access))
}

func (s *server) Logout(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.SessionFrom(r.Context())
	if err := s.o.Auth.Logout(r.Context(), sess.ID); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.signIn(r, "logout", nil, "", "", nil)
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) GetMe(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.SessionFrom(r.Context())
	writeJSON(w, http.StatusOK, s.describe(r, sess, authz.From(r.Context())))
}

func (s *server) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var c gen.PasswordChange
	if !decode(w, r, &c) {
		return
	}
	sess, _ := auth.SessionFrom(r.Context())
	err := s.o.Auth.ChangePassword(r.Context(), sess, c.CurrentPassword, c.NewPassword)
	ev := seclog.Event{Category: seclog.Change, Kind: "account.password", Outcome: seclog.Success, Target: sess.Username}
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		ev.Outcome, ev.Reason = seclog.Failure, "wrong_current_password"
	case errors.Is(err, auth.ErrWeakPassword):
		ev.Outcome, ev.Reason = seclog.Failure, "weak_password"
	}
	if err == nil || ev.Outcome == seclog.Failure {
		s.secRecord(r, ev)
	}
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{
			{Field: "current_password", Code: "wrong", Message: "current password does not match"}})
	case errors.Is(err, auth.ErrWeakPassword):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{
			{Field: "new_password", Code: "weak", Message: err.Error()}})
	default:
		writeError(w, r, s.o.Logger, err)
	}
}
