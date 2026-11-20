// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The Prüfweg of PL77: an account with a one-time password must change it at the first
// sign-in; an invited one sets its password with the link of the mail, once, and not after the
// link expired; a blocked one cannot sign in; without manage_rights 403; the security log names
// every change.
func TestAccountsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	stamp := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	signIn := func(name, password string) (int, *testAPI) {
		t.Helper()
		r := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "`+password+`"}`, map[string]string{"Cookie": ""})
		if r.code != 200 {
			return r.code, nil
		}
		return r.code, &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, r.hdr)}
	}

	// One-time password.
	once := "acc_once_" + stamp
	r := call(t, admin, http.MethodPost, "/api/v1/accounts", `{"username": "`+once+`", "first_name": "Erna", "last_name": "Einmal", "password": "Einmal-Pw7!x"}`, nil)
	if r.code != 201 || r.body["password_expired"] != true || r.body["origin"] != "local" || r.body["person_id"] == nil || r.body["display_name"] != "Erna Einmal" {
		t.Fatalf("one-time: %d %s", r.code, r.raw)
	}
	onceID := r.body["id"].(string)
	if r := call(t, admin, http.MethodPost, "/api/v1/accounts", `{"username": "`+once+`", "last_name": "Doppelt", "password": "Einmal-Pw7!x"}`, nil); r.code != 409 {
		t.Fatalf("taken name: %d %s", r.code, r.raw)
	}
	code, user := signIn(once, "Einmal-Pw7!x")
	if code != 200 {
		t.Fatalf("sign-in with the one-time password: %d", code)
	}
	if r := call(t, user, http.MethodGet, "/api/v1/types", "", nil); r.code != 403 {
		t.Fatalf("before the change the password blocks: %d", r.code)
	}
	if r := call(t, user, http.MethodPost, "/api/v1/auth/password", `{"current_password": "Einmal-Pw7!x", "new_password": "Eigenes-Pw8!y"}`, nil); r.code != 204 {
		t.Fatalf("change: %d %s", r.code, r.raw)
	}
	if r := call(t, user, http.MethodGet, "/api/v1/types", "", nil); r.code != 200 {
		t.Fatalf("after the change: %d", r.code)
	}

	// Invitation by mail.
	invited := "acc_invite_" + stamp
	address := invited + "@example.test"
	r = call(t, admin, http.MethodPost, "/api/v1/accounts", `{"username": "`+invited+`", "last_name": "Eingeladen", "email": "`+address+`"}`, nil)
	if r.code != 201 || r.body["invited_until"] == nil || r.body["invited_email"] != address {
		t.Fatalf("invite: %d %s", r.code, r.raw)
	}
	invitedID := r.body["id"].(string)
	if code, _ := signIn(invited, "Aa1!irgendwas"); code != 401 {
		t.Fatalf("an invited account cannot sign in yet: %d", code)
	}
	// keyOf waits for the mail number n to the address and reads the key of its link.
	keyOf := func(n int) string {
		t.Helper()
		re := regexp.MustCompile(`/invite\?key=([A-Za-z0-9_-]+)`)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var keys []string
			for _, m := range admin.smtp.Mail() {
				if slices.Contains(m.To, address) {
					if k := re.FindStringSubmatch(m.Body); k != nil {
						keys = append(keys, k[1])
					}
				}
			}
			if len(keys) >= n {
				return keys[n-1]
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("no mail number %d with the link", n)
		return ""
	}
	key := keyOf(1)
	public := map[string]string{"Cookie": ""}
	if r := call(t, admin, http.MethodPost, "/api/v1/invitations:accept", `{"key": "`+key+`", "password": "weak"}`, public); r.code != 422 {
		t.Fatalf("weak password: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/invitations:accept", `{"key": "`+key+`", "password": "Gesetzt-Pw9!z"}`, public); r.code != 200 || r.body["username"] != invited {
		t.Fatalf("accept: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/invitations:accept", `{"key": "`+key+`", "password": "Gesetzt-Pw9!z"}`, public); r.code != 422 {
		t.Fatalf("a key holds once: %d %s", r.code, r.raw)
	}
	if code, _ := signIn(invited, "Gesetzt-Pw9!z"); code != 200 {
		t.Fatalf("sign-in after the invitation: %d", code)
	}

	// A new invitation that expired (fixed clock: its end moved to the past).
	if r := call(t, admin, http.MethodPost, "/api/v1/accounts/"+invitedID+":invite", `{}`, nil); r.code != 204 {
		t.Fatalf("invite again: %d %s", r.code, r.raw)
	}
	key = keyOf(2)
	if _, err := admin.pool.Exec(ctx, `UPDATE account_invitation SET expires_at = now() - interval '1 minute' WHERE account_id = $1`, uuid.MustParse(invitedID)); err != nil {
		t.Fatal(err)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/invitations:accept", `{"key": "`+key+`", "password": "Spaeter-Pw9!z"}`, public); r.code != 422 {
		t.Fatalf("an expired key: %d %s", r.code, r.raw)
	}

	// The mail server refuses the address: the account is there, without an invitation, and the
	// answer says so; a new invitation to it answers 502 and leaves none either.
	refused := "acc_refused_" + stamp
	admin.smtp.Refuse(refused + "@example.test")
	r = call(t, admin, http.MethodPost, "/api/v1/accounts", `{"username": "`+refused+`", "last_name": "Abgewiesen", "email": "`+refused+`@example.test"}`, nil)
	if r.code != 201 || r.body["invitation_failed"] != true || r.body["invited_until"] != nil {
		t.Fatalf("a refused invitation: %d %s", r.code, r.raw)
	}
	refusedID := r.body["id"].(string)
	if r := call(t, admin, http.MethodPost, "/api/v1/accounts/"+refusedID+":invite", `{}`, nil); r.code != 502 {
		t.Fatalf("invite again, refused: %d %s", r.code, r.raw)
	}
	if n := count(t, admin.pool, "SELECT count(*) FROM account_invitation WHERE account_id = $1", uuid.MustParse(refusedID)); n != 0 {
		t.Fatalf("invitations left after refused mails: %d", n)
	}

	// Blocked, then allowed again; a new one-time password.
	if r := call(t, admin, http.MethodPost, "/api/v1/accounts/"+onceID+":disable", "", nil); r.code != 204 {
		t.Fatalf("disable: %d %s", r.code, r.raw)
	}
	if code, _ := signIn(once, "Eigenes-Pw8!y"); code != 401 {
		t.Fatalf("blocked: %d", code)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/accounts/"+onceID+":enable", "", nil); r.code != 204 {
		t.Fatalf("enable: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/accounts/"+onceID+":password", `{"password": "Neu-Einmal7!q"}`, nil); r.code != 204 {
		t.Fatalf("one-time again: %d %s", r.code, r.raw)
	}
	list := call(t, admin, http.MethodGet, "/api/v1/accounts", "", nil)
	if list.code != 200 || !strings.Contains(list.raw, `"username":"`+once+`"`) {
		t.Fatalf("list: %d %s", list.code, list.raw)
	}
	var accounts []map[string]any
	if err := json.Unmarshal([]byte(list.raw), &accounts); err != nil {
		t.Fatal(err)
	}
	for _, it := range accounts {
		if it["username"] == once && (it["active"] != true || it["password_expired"] != true || it["last_login_at"] == nil) {
			t.Fatalf("listed: %v", it)
		}
	}

	// Without manage_rights.
	if r := call(t, user, http.MethodGet, "/api/v1/accounts", "", nil); r.code != 403 {
		t.Fatalf("without the right: %d", r.code)
	}

	for _, kind := range []string{"account.create", "account.invite", "account.invitation_used", "account.disable", "account.enable", "account.password_reset"} {
		if count(t, admin.pool, "SELECT count(*) FROM security_event WHERE kind = $1 AND (target = $2 OR target = $3)", kind, once, invited) == 0 {
			t.Fatalf("security log without %s", kind)
		}
	}
}
