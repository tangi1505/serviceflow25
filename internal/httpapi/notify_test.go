// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/notify/smtptest"
)

// The Prüfweg of WF15: a new record sends a mail to each member of its assigned group, in
// German or English as each person has set, with the placeholders filled in; the person who
// created it gets none, a person without an address none, a person named twice one. A rule's
// notify goes out as mail with its message. Wrong notifications are refused.
func TestNotificationsOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	title := "WF15 " + suffix
	var made []string
	defer func() {
		call(t, api, http.MethodDelete, "/api/v1/rules/wf15_check", "", nil)
		for i := len(made) - 1; i >= 0; i-- {
			typ, id, _ := strings.Cut(made[i], "/")
			call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+id, "", nil)
			call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+id+":purge", "", nil)
		}
	}()
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		id := r.body["id"].(string)
		made = append(made, typeName+"/"+id)
		return id
	}
	anna := create("core.person", `{"first_name": "Anna", "last_name": "WF15 `+suffix+`", "email": "anna-`+suffix+`@example.org", "language": "de"}`)
	ben := create("core.person", `{"first_name": "Ben", "last_name": "WF15 `+suffix+`", "email": "ben-`+suffix+`@example.org", "language": "en"}`)
	nomail := create("core.person", `{"first_name": "Ohne", "last_name": "WF15 `+suffix+`"}`)
	group := create("core.group", `{"name": "WF15 Netz `+suffix+`"}`)
	for _, p := range []string{anna, ben, nomail, api.person.String()} {
		create("core.group_member", `{"group": "`+group+`", "person": "`+p+`"}`)
	}

	// Wrong notifications: an unknown event, a recipient that is none, a placeholder of no field.
	for _, c := range []struct{ body, field string }{
		{`"events": "fly", "recipients": "actors.assigned", "subject": "x", "body": "x"`, "events"},
		{`"events": "create", "recipients": "jemand", "subject": "x", "body": "x"`, "recipients"},
		{`"events": "create", "recipients": "actors.nobody", "subject": "x", "body": "x"`, "recipients"},
		{`"events": "create", "recipients": "actors.assigned", "subject": "{{record.nofield}}", "body": "x"`, "subject"},
		{`"events": "create", "recipients": "actors.assigned", "subject": "x", "body": "{{whatever}}"`, "body"},
	} {
		r := call(t, api, http.MethodPost, "/api/v1/records/core.notification", `{"name": "WF15 falsch `+suffix+`", "target_type": "probe.probe", `+c.body+`}`, nil)
		if r.code != 422 || !strings.Contains(r.raw, `"`+c.field+`"`) {
			t.Fatalf("wrong notification %s: %d %s", c.body, r.code, r.raw)
		}
	}
	// To the assigned group and, named twice, to Anna; German in the column, English as a
	// translation.
	n := create("core.notification", `{"name": "WF15 Neu `+suffix+`", "target_type": "probe.probe", "events": "create",
		"recipients": "actors.assigned, core.person:`+anna+`",
		"subject": "Neuer Vorgang: {{record.title}}", "body": "{{type}} „{{record.title}}“ (Art {{record.kind}}) von {{actor}}.\n{{link}}"}`)
	if r := call(t, api, http.MethodPatch, "/api/v1/records/core.notification/"+n,
		`{"subject": "New record: {{record.title}}", "body": "{{type}} \"{{record.title}}\" (kind {{record.kind}}) by {{actor}}.\n{{link}}"}`,
		map[string]string{"If-Match": `"1"`, "Content-Language": "en"}); r.code != 200 {
		t.Fatalf("English: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/records/core.notification/"+n, "", map[string]string{"Accept-Language": "en"}); r.body["subject"] != "New record: {{record.title}}" {
		t.Fatalf("English is not kept: %s", r.raw)
	}

	id := create("probe.probe", `{"title": "`+title+` Router", "kind": "fault", "actors": {"assigned": [{"type": "core.group", "id": "`+group+`"}]}}`)
	mails := waitForMail(t, api, 2, func(m smtptest.Mail) bool { return strings.Contains(m.Subject, title) })
	byTo := map[string]smtptest.Mail{}
	for _, m := range mails {
		byTo[m.To[0]] = m
	}
	de, en := byTo["anna-"+suffix+"@example.org"], byTo["ben-"+suffix+"@example.org"]
	link := "http://itsm.test/types/probe.probe/" + id
	if de.Subject != "Neuer Vorgang: "+title+" Router" || !strings.Contains(de.Body, "„"+title+" Router“ (Art Störung) von ") ||
		!strings.Contains(de.Body, link) || de.From != "itsm@test.local" {
		t.Fatalf("German mail: %+v", de)
	}
	if en.Subject != "New record: "+title+" Router" || !strings.Contains(en.Body, `"`+title+` Router" (kind Fault) by `) || !strings.Contains(en.Body, link) {
		t.Fatalf("English mail: %+v", en)
	}

	// A rule's notify goes out as mail with its message, to the person it names.
	if r := call(t, api, http.MethodPut, "/api/v1/rules/wf15_check", `{"type_name": "probe.probe", "trigger": "create", "mode": "after",
		"condition": "record.title.startsWith(\"`+title+` Regel\")", "actions": [{"notify": {"to": ["core.person:`+ben+`"], "message": "Bitte prüfen"}}]}`, nil); r.code != 201 {
		t.Fatalf("rule: %d %s", r.code, r.raw)
	}
	create("probe.probe", `{"title": "`+title+` Regel"}`)
	deadline := time.Now().Add(20 * time.Second)
	for {
		if err := api.ruleEvents.Dispatch(ctx); err != nil {
			t.Fatal(err)
		}
		if err := api.notifyEvents.Dispatch(ctx); err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(api.smtp.Mail(), func(m smtptest.Mail) bool {
			return m.To[0] == "ben-"+suffix+"@example.org" && strings.Contains(m.Body, "Bitte prüfen")
		})
		if i >= 0 {
			if m := api.smtp.Mail()[i]; !strings.Contains(m.Subject, title+" Regel") {
				t.Fatalf("mail of the rule: %+v", m)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no mail of the rule: %+v", api.smtp.Mail())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitForMail dispatches until want mails that match have arrived, then a moment longer to
// see that no more come; it fails on more or fewer.
func waitForMail(t *testing.T, api *testAPI, want int, match func(smtptest.Mail) bool) []smtptest.Mail {
	t.Helper()
	ctx := t.Context()
	got := func() []smtptest.Mail {
		var out []smtptest.Mail
		for _, m := range api.smtp.Mail() {
			if match(m) {
				out = append(out, m)
			}
		}
		return out
	}
	deadline := time.Now().Add(20 * time.Second)
	for len(got()) < want {
		// The dispatcher of every database (PL47c), as the worker runs them.
		for _, d := range api.rt.Databases() {
			if err := api.notifyEvents.Dispatch(d.Bind(ctx)); err != nil {
				t.Fatal(err)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d mails: %+v", len(got()), want, api.smtp.Mail())
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(time.Second)
	if mails := got(); len(mails) != want {
		t.Fatalf("%d mails, want %d: %+v", len(mails), want, mails)
	}
	return got()
}

// TestInboxOverHTTP is the acceptance of WF19: a notification with the way inbox leaves an
// entry for the person, in her language, without a mail; one with mail only leaves none; the
// open stream of the person hears of it; read, all read, remove; old entries go with the job.
func TestInboxOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	title := "WF19 " + suffix
	var made []string
	defer func() {
		for i := len(made) - 1; i >= 0; i-- {
			typ, id, _ := strings.Cut(made[i], "/")
			call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+id, "", nil)
			call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+id+":purge", "", nil)
		}
		_, _ = api.pool.Exec(context.Background(), "DELETE FROM inbox_entry WHERE subject LIKE $1", "%"+title+"%")
	}()
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		id := r.body["id"].(string)
		made = append(made, typeName+"/"+id)
		return id
	}
	anna := create("core.person", `{"first_name": "Anna", "last_name": "WF19 `+suffix+`", "email": "anna-`+suffix+`@example.org", "language": "de"}`)
	// Into the inbox of the session's person, who causes the event herself; and as mail only to Anna.
	n := create("core.notification", `{"name": "WF19 Eingang `+suffix+`", "target_type": "probe.probe", "events": "create",
		"recipients": "core.person:`+api.person.String()+`", "notify_actor": true, "mail": false,
		"subject": "Eingang: {{record.title}}", "body": "{{type}} von {{actor}}"}`)
	if r := call(t, api, http.MethodGet, "/api/v1/records/core.notification/"+n, "", nil); r.body["inbox"] != true || r.body["mail"] != false {
		t.Fatalf("ways: %s", r.raw)
	}
	create("core.notification", `{"name": "WF19 Mail `+suffix+`", "target_type": "probe.probe", "events": "create",
		"recipients": "core.person:`+anna+`", "inbox": false, "subject": "Mail: {{record.title}}", "body": "{{link}}"}`)

	before := call(t, api, http.MethodGet, "/api/v1/me/inbox?unread=true&limit=1", "", nil)
	if before.code != 200 {
		t.Fatalf("inbox: %d %s", before.code, before.raw)
	}
	unreadBefore := int(before.body["unread"].(float64))

	// The stream of the session is open before the record comes.
	sctx, stop := context.WithCancel(ctx)
	defer stop()
	req, err := http.NewRequestWithContext(sctx, http.MethodGet, api.URL+"/api/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", sessionCookie+"="+api.cookie)
	res, err := http.DefaultClient.Do(req) //nolint:bodyclose // closed below, after the stream was read
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	lines := make(chan string, 256)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	for l := range lines {
		if l == ": connected" {
			break
		}
	}

	id := create("probe.probe", `{"title": "`+title+` Drucker"}`)
	var entry map[string]any
	deadline := time.Now().Add(20 * time.Second)
	for entry == nil {
		for _, d := range api.rt.Databases() {
			if err := api.notifyEvents.Dispatch(d.Bind(ctx)); err != nil {
				t.Fatal(err)
			}
		}
		r := call(t, api, http.MethodGet, "/api/v1/me/inbox?unread=true", "", nil)
		for _, it := range r.body["items"].([]any) {
			if m := it.(map[string]any); strings.Contains(m["subject"].(string), title) {
				entry = m
			}
		}
		if entry == nil && time.Now().After(deadline) {
			t.Fatalf("no entry: %s", r.raw)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if entry["subject"] != "Eingang: "+title+" Drucker" || entry["kind"] != "create" || entry["type_name"] != "probe.probe" ||
		entry["record_id"] != id || entry["notification_id"] != n || entry["read_at"] != nil || entry["body"] != "Probe von API Test" || entry["actor"] != "API Test" {
		t.Fatalf("entry: %v", entry)
	}
	// The stream tells the number of the unread.
	want := fmt.Sprintf(`data: {"unread":%d}`, unreadBefore+1)
	got := false
	for !got {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("stream closed")
			}
			got = l == want
		case <-time.After(10 * time.Second):
			t.Fatalf("no %s on the stream", want)
		}
	}
	// Anna gets her mail and no entry; the way mail does not fill the inbox.
	waitForMail(t, api, 1, func(m smtptest.Mail) bool { return strings.Contains(m.Subject, title) })
	var annas int
	if err := api.pool.QueryRow(ctx, "SELECT count(*) FROM inbox_entry WHERE person_id = $1", anna).Scan(&annas); err != nil {
		t.Fatal(err)
	}
	if annas != 0 {
		t.Fatalf("Anna has %d entries, wants mail only", annas)
	}

	// Read, then remove; another's entry or an unknown one is 404, nothing named is 422.
	if r := call(t, api, http.MethodPost, "/api/v1/me/inbox:read", `{"ids": ["`+entry["id"].(string)+`"]}`, nil); r.code != 200 || int(r.body["unread"].(float64)) != unreadBefore {
		t.Fatalf("read: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/me/inbox?unread=true", "", nil); strings.Contains(r.raw, title) {
		t.Fatalf("read entry still unread: %s", r.raw)
	}
	if r := call(t, api, http.MethodPost, "/api/v1/me/inbox:read", `{}`, nil); r.code != 422 {
		t.Fatalf("read nothing: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodDelete, "/api/v1/me/inbox/"+entry["id"].(string), "", nil); r.code != 204 {
		t.Fatalf("remove: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodDelete, "/api/v1/me/inbox/"+entry["id"].(string), "", nil); r.code != 404 {
		t.Fatalf("remove twice: %d %s", r.code, r.raw)
	}

	// Paging and all read: three entries, two per page; all read leaves none unread.
	for i := range 3 {
		if _, err := api.pool.Exec(ctx, `INSERT INTO inbox_entry (id, person_id, event_position, kind, type_name, record_id, subject, body, created_at)
VALUES ($1, $2, $3, 'create', 'probe.probe', $4, $5, '', now() - make_interval(secs => $6))`,
			uuid.New(), api.person, -1-i, id, fmt.Sprintf("%s Seite %d", title, i), i); err != nil {
			t.Fatal(err)
		}
	}
	first := call(t, api, http.MethodGet, "/api/v1/me/inbox?limit=2", "", nil)
	next, _ := first.body["next"].(string)
	second := call(t, api, http.MethodGet, "/api/v1/me/inbox?limit=2&after="+next, "", nil)
	if !strings.Contains(first.raw, title+" Seite 0") || !strings.Contains(first.raw, title+" Seite 1") || next == "" ||
		!strings.Contains(second.raw, title+" Seite 2") || strings.Contains(second.raw, title+" Seite 1") {
		t.Fatalf("pages:\n%s\n%s", first.raw, second.raw)
	}
	if r := call(t, api, http.MethodPost, "/api/v1/me/inbox:read", `{"all": true}`, nil); r.code != 200 || r.body["unread"] != float64(0) {
		t.Fatalf("all read: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/me/inbox?after=nix", "", nil); r.code != 422 {
		t.Fatalf("bad after: %d %s", r.code, r.raw)
	}

	// Retention: an entry of 91 days goes, one of 89 stays (default 90).
	old, young := uuid.New(), uuid.New()
	for _, e := range []struct {
		id   uuid.UUID
		days int
	}{{old, 91}, {young, 89}} {
		if _, err := api.pool.Exec(ctx, `INSERT INTO inbox_entry (id, person_id, event_position, kind, type_name, record_id, subject, body, created_at)
VALUES ($1, $2, $3, 'create', 'probe.probe', $4, $5, '', now() - make_interval(days => $6))`,
			e.id, api.person, -10-e.days, id, title+" alt", e.days); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range api.rt.Databases() {
		if _, err := api.notify.ExpireInbox(d.Bind(ctx), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	var left []uuid.UUID
	rows, err := api.pool.Query(ctx, "SELECT id FROM inbox_entry WHERE id = ANY($1)", []uuid.UUID{old, young})
	if err != nil {
		t.Fatal(err)
	}
	if left, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID]); err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0] != young {
		t.Fatalf("after the retention: %v, want only %s", left, young)
	}
}
