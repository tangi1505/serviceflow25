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
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seclog"
)

// --- accounts in the interface (PL77, UI-Plan E63): manage_rights on core.role ---

// accountJSON is an account as GET /api/v1/accounts shows it.
type accountJSON struct {
	ID              uuid.UUID  `json:"id"`
	Username        string     `json:"username"`
	DisplayName     string     `json:"display_name"`
	Origin          string     `json:"origin"`
	Issuer          string     `json:"issuer,omitempty"`
	Tenant          string     `json:"tenant,omitempty"`
	PersonID        *uuid.UUID `json:"person_id,omitempty"`
	Active          bool       `json:"active"`
	PasswordExpired bool       `json:"password_expired"`
	NoPassword      bool       `json:"no_password,omitempty"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	InvitedUntil    *time.Time `json:"invited_until,omitempty"`
	InvitedEmail    string     `json:"invited_email,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	// InvitationFailed is set only in the answer of creating: the account is there, but the
	// mail of the invitation did not go out.
	InvitationFailed bool `json:"invitation_failed,omitempty"`
}

func toAccountJSON(a auth.AccountInfo, now time.Time) accountJSON {
	return accountJSON{
		ID: a.ID, Username: a.Username, DisplayName: a.DisplayName, Origin: a.Origin, Issuer: a.Issuer,
		Tenant: a.Tenant, PersonID: a.PersonID, Active: a.DisabledAt == nil,
		PasswordExpired: a.Origin == "local" && a.PasswordExpired(now),
		NoPassword:      a.Origin == "local" && a.PasswordUnknown,
		LastLoginAt:     a.LastLoginAt, InvitedUntil: a.InvitedUntil, InvitedEmail: a.InvitedEmail,
		CreatedAt: a.CreatedAt,
	}
}

// ownAccount reports whether an account belongs to the session's tenant, as for clients:
// one of the installation needs manage_rights at the root (PL45).
func (s *server) ownAccount(r *http.Request, tenant string) bool {
	a := authz.From(r.Context())
	if !a.Bounded() {
		return true
	}
	if tenant == "" {
		return a.Installation(rightsType, "manage_rights")
	}
	return tenant == a.CurrentTenant()
}

func (s *server) ListAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.mayManageRights(w, r) {
		return
	}
	list, err := s.o.Auth.Accounts(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	now := time.Now()
	out := []accountJSON{}
	for _, a := range list {
		if s.ownAccount(r, a.Tenant) {
			out = append(out, toAccountJSON(a, now))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// oneAccount reads the account of the path; another tenant's is 404.
func (s *server) oneAccount(w http.ResponseWriter, r *http.Request, id gen.Id) (auth.Account, bool) {
	if !s.mayManageRights(w, r) {
		return auth.Account{}, false
	}
	acc, err := s.o.Auth.AccountByID(r.Context(), id)
	if err == nil && !s.ownAccount(r, acc.Tenant) {
		err = auth.ErrAccountNotFound
	}
	if err != nil {
		if errors.Is(err, auth.ErrAccountNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "no such account", nil)
		} else {
			writeError(w, r, s.o.Logger, err)
		}
		return auth.Account{}, false
	}
	return acc, true
}

func (s *server) CreateAccount(w http.ResponseWriter, r *http.Request) {
	if !s.mayManageRights(w, r) {
		return
	}
	var body gen.AccountCreate
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	var problems []records.Problem
	if !auth.ValidUsername(body.Username) {
		problems = append(problems, records.Problem{Field: "username", Code: "invalid", Message: auth.ErrInvalidUsername.Error()})
	}
	if strings.TrimSpace(body.LastName) == "" {
		problems = append(problems, records.Problem{Field: "last_name", Code: "required", Message: "the last name is needed"})
	}
	email := ""
	if body.Email != nil {
		email = strings.TrimSpace(*body.Email)
	}
	invite := body.Password == nil || *body.Password == ""
	if invite && email == "" {
		problems = append(problems, records.Problem{Field: "email", Code: "required", Message: "an invitation needs an address; or give a one-time password"})
	}
	if len(problems) > 0 {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", problems)
		return
	}
	if invite && !s.mailReady(w, r) {
		return
	}
	display := strings.TrimSpace(body.LastName)
	if body.FirstName != nil && strings.TrimSpace(*body.FirstName) != "" {
		display = strings.TrimSpace(*body.FirstName) + " " + display
	}
	tenant := ""
	if a := authz.From(ctx); a.Bounded() {
		tenant = a.CurrentTenant()
	}
	var acc auth.Account
	var err error
	if invite {
		acc, err = s.o.Auth.CreateInvitedAccount(ctx, tenant, body.Username, display)
	} else {
		now := time.Now()
		acc, err = s.o.Auth.CreateAccountIn(ctx, tenant, body.Username, display, *body.Password, &now)
	}
	if !s.accountWritten(w, r, err) {
		return
	}
	// The person of the account (E82), then the link; the person carries name and address.
	person := map[string]any{"last_name": strings.TrimSpace(body.LastName)}
	if body.FirstName != nil && strings.TrimSpace(*body.FirstName) != "" {
		person["first_name"] = strings.TrimSpace(*body.FirstName)
	}
	if email != "" {
		person["email"] = email
	}
	if body.OrgPath != nil && *body.OrgPath != "" {
		person["org_path"] = *body.OrgPath
	}
	rec, err := s.o.Records.Create(ctx, "core.person", person, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	pid, ok := rec["id"].(uuid.UUID)
	if !ok {
		pid, err = uuid.Parse(fmt.Sprint(rec["id"]))
		if err != nil {
			writeError(w, r, s.o.Logger, err)
			return
		}
	}
	if err := s.o.Authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	acc.PersonID = &pid
	kind := "account.create"
	details := map[string]any{"one_time_password": !invite}
	failed := false
	if invite {
		kind = "account.invite"
		if err := s.sendInvitation(r, acc, email); err != nil {
			// The account stays (it has a person and maybe roles soon); the invitation without
			// its mail goes, so the page shows no waiting invitation that nobody received.
			s.o.Logger.Warn("invitation mail failed", "account", acc.Username, "err", err)
			if err := s.o.Auth.WithdrawInvitation(ctx, acc); err != nil {
				writeError(w, r, s.o.Logger, err)
				return
			}
			kind, failed = "account.create", true
			details["invitation_failed"] = true
		}
	}
	s.secRecord(r, seclog.Event{Category: seclog.Change, Kind: kind, Outcome: seclog.Success, Target: acc.Username,
		Details: details})
	list, err := s.o.Auth.Accounts(ctx)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	out := toAccountJSON(auth.AccountInfo{Account: acc, Origin: "local"}, time.Now())
	for _, a := range list {
		if a.ID == acc.ID {
			out = toAccountJSON(a, time.Now())
		}
	}
	out.InvitationFailed = failed
	writeJSON(w, http.StatusCreated, out)
}

// accountWritten answers the errors of creating an account or setting its password.
func (s *server) accountWritten(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, auth.ErrAccountExists):
		writeProblem(w, r, http.StatusConflict, "conflict", "the username is taken", nil)
	case errors.Is(err, auth.ErrInvalidUsername):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{{Field: "username", Code: "invalid", Message: err.Error()}})
	case errors.Is(err, auth.ErrWeakPassword):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{{Field: "password", Code: "weak", Message: err.Error()}})
	default:
		writeError(w, r, s.o.Logger, err)
	}
	return false
}

// mailReady answers 503 when invitations cannot go out.
func (s *server) mailReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Notify == nil || !s.o.Notify.MailEnabled() {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "invitations need a mail server (ITSM_SMTP_HOST); give a one-time password instead", nil)
		return false
	}
	return true
}

// errInvitationMail is a mail of an invitation that did not go out.
var errInvitationMail = errors.New("the mail of the invitation did not go out")

// sendInvitation makes a new invitation and mails its link; a mail that does not go out
// answers errInvitationMail, the invitation itself stays (the caller decides).
func (s *server) sendInvitation(r *http.Request, acc auth.Account, email string) error {
	var by *uuid.UUID
	if sess, ok := auth.SessionFrom(r.Context()); ok {
		id := sess.AccountID
		by = &id
	}
	key, err := s.o.Auth.Invite(r.Context(), acc, email, by)
	if err != nil {
		return err
	}
	link := strings.TrimRight(s.o.Notify.PublicURL, "/") + "/invite?key=" + key
	subject := "Einladung zu ServiceFlow25"
	text := fmt.Sprintf("Guten Tag %s,\n\nfür Sie ist ein Konto angelegt, Benutzername: %s.\n"+
		"Mit diesem Link setzen Sie Ihr Kennwort (gültig bis %s):\n\n%s\n\n"+
		"Der Link gilt einmal. Haben Sie kein Konto erwartet, ignorieren Sie diese Mail.\n",
		acc.DisplayName, acc.Username, time.Now().Add(auth.InvitationValid).In(berlin()).Format("02.01.2006 15:04"), link)
	if err := s.o.Notify.MailNow(r.Context(), email, subject, text); err != nil {
		return fmt.Errorf("%w: %w", errInvitationMail, err)
	}
	return nil
}

func berlin() *time.Location {
	if loc, err := time.LoadLocation("Europe/Berlin"); err == nil {
		return loc
	}
	return time.UTC
}

func (s *server) DisableAccount(w http.ResponseWriter, r *http.Request, id gen.Id) {
	s.setDisabled(w, r, id, true)
}

func (s *server) EnableAccount(w http.ResponseWriter, r *http.Request, id gen.Id) {
	s.setDisabled(w, r, id, false)
}

func (s *server) setDisabled(w http.ResponseWriter, r *http.Request, id gen.Id, disabled bool) {
	acc, ok := s.oneAccount(w, r, id)
	if !ok {
		return
	}
	if sess, ok := auth.SessionFrom(r.Context()); ok && disabled && sess.AccountID == acc.ID {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{{Field: "id", Code: "self", Message: "one's own account cannot be blocked"}})
		return
	}
	if err := s.o.Auth.SetDisabled(r.Context(), acc.Username, disabled); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	kind := "account.enable"
	if disabled {
		kind = "account.disable"
	}
	s.secRecord(r, seclog.Event{Category: seclog.Change, Kind: kind, Outcome: seclog.Success, Target: acc.Username})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) SetAccountPassword(w http.ResponseWriter, r *http.Request, id gen.Id) {
	acc, ok := s.oneAccount(w, r, id)
	if !ok {
		return
	}
	var body gen.OneTimePassword
	if !decode(w, r, &body) {
		return
	}
	if !s.accountWritten(w, r, s.o.Auth.SetOneTimePassword(r.Context(), acc, body.Password)) {
		return
	}
	s.secRecord(r, seclog.Event{Category: seclog.Change, Kind: "account.password_reset", Outcome: seclog.Success, Target: acc.Username})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) InviteAccount(w http.ResponseWriter, r *http.Request, id gen.Id) {
	acc, ok := s.oneAccount(w, r, id)
	if !ok {
		return
	}
	var body gen.Invitation
	if !decode(w, r, &body) {
		return
	}
	// The mail server first: without one, no address helps.
	if !s.mailReady(w, r) {
		return
	}
	email := ""
	if body.Email != nil {
		email = strings.TrimSpace(*body.Email)
	}
	if email == "" && acc.PersonID != nil {
		if p, err := s.o.Records.Get(r.Context(), "core.person", *acc.PersonID); err == nil {
			email, _ = p["email"].(string)
		}
	}
	if email == "" {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{{Field: "email", Code: "required", Message: "the person has no address; give one"}})
		return
	}
	if err := s.sendInvitation(r, acc, email); err != nil {
		if !errors.Is(err, errInvitationMail) {
			writeError(w, r, s.o.Logger, err)
			return
		}
		// The new invitation replaced the old one; without its mail it goes too.
		s.o.Logger.Warn("invitation mail failed", "account", acc.Username, "err", err)
		if err := s.o.Auth.WithdrawInvitation(r.Context(), acc); err != nil {
			writeError(w, r, s.o.Logger, err)
			return
		}
		writeProblem(w, r, http.StatusBadGateway, "mail_failed", "the mail server did not take the invitation; try again later or give a one-time password", nil)
		return
	}
	s.secRecord(r, seclog.Event{Category: seclog.Change, Kind: "account.invite", Outcome: seclog.Success, Target: acc.Username})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var body gen.InvitationAccept
	if !decode(w, r, &body) {
		return
	}
	username, err := s.o.Auth.AcceptInvitation(r.Context(), body.Key, body.Password)
	switch {
	case err == nil:
		s.secRecord(r, seclog.Event{Category: seclog.Change, Kind: "account.invitation_used", Outcome: seclog.Success, Target: username, Username: username})
		writeJSON(w, http.StatusOK, map[string]string{"username": username})
	case errors.Is(err, auth.ErrInvitationInvalid):
		s.secRecord(r, seclog.Event{Category: seclog.Change, Kind: "account.invitation_used", Outcome: seclog.Failure, Reason: "invalid_key"})
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{{Field: "key", Code: "invalid", Message: err.Error()}})
	case errors.Is(err, auth.ErrWeakPassword):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{{Field: "password", Code: "weak", Message: err.Error()}})
	default:
		writeError(w, r, s.o.Logger, err)
	}
}
