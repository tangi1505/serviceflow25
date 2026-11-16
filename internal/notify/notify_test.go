// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package notify

import (
	"slices"
	"strings"
	"testing"
	"time"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/notify/smtptest"
)

var probe = meta.Type{Name: "probe", Package: "probe", TitleField: "title",
	Label: meta.Label{"de": "Vorgang", "en": "Record"},
	Fields: []meta.Field{{Name: "title", Type: meta.FieldText}, {Name: "kind", Type: meta.FieldChoice,
		Options: []meta.Option{{Value: "fault", Label: meta.Label{"de": "Störung", "en": "Fault"}}}}},
	Actors: []meta.ActorRole{{Name: "assigned"}}}

func TestRender(t *testing.T) {
	d := Data{Type: probe, Record: map[string]any{"title": "Router", "kind": "fault", "state": "new"}, Lang: "en",
		Link: "http://itsm/x", Actor: "Anna", Message: "Bitte prüfen", Entry: "Neu"}
	got := Render("{{type}}: {{title}} ({{ record.kind }}, {{record.state}}) by {{actor}} {{link}} {{message}} {{entry}} {{record.gone}} {{other}}", d)
	// A placeholder the check would refuse (it can only hide in a translation) stays as it is.
	if want := "Record: Router (Fault, new) by Anna http://itsm/x Bitte prüfen Neu {{record.gone}} {{other}}"; got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	d.Lang = "fr" // no label in that language: German
	if got := Render("{{type}} {{record.kind}}", d); got != "Vorgang Störung" {
		t.Fatalf("fallback: %q", got)
	}
}

// In formatted text (the text modules, SD8) the values are escaped, {{link}} is a link and a
// reference shows the title Name gives.
func TestRenderHTML(t *testing.T) {
	typ := probe
	typ.Fields = append(slices.Clone(probe.Fields), meta.Field{Name: "owner", Type: meta.FieldReference, To: "core.person"})
	d := Data{Type: typ, Record: map[string]any{"title": "Router & <Switch>", "owner": "p1"}, Lang: "de",
		Link: "http://itsm/x?a=1&b=2", Actor: "Anna", HTML: true,
		Name: func(to, id string) string { return to + "/" + id }}
	got := Render("<p>{{title}} {{record.owner}} {{link}}</p>", d)
	want := `<p>Router &amp; &lt;Switch&gt; core.person/p1 <a href="http://itsm/x?a=1&amp;b=2">http://itsm/x?a=1&amp;b=2</a></p>`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if msg := CheckPlaceholders("{{record.owner}} {{message}}", []string{"title", "actor"}, "", func(string) bool { return true }); !strings.Contains(msg, "{{title}} or {{actor}}") {
		t.Fatalf("names: %q", msg)
	}
}

func TestCheck(t *testing.T) {
	for text, ok := range map[string]bool{
		"{{record.title}} {{type}} {{title}} {{link}} {{actor}} {{message}} {{entry}} {{record.id}} {{record.state}}": true,
		"{{record.nofield}}": false, "{{whatever}}": false, "no placeholder at all": true,
	} {
		if msg := checkPlaceholders(probe, text); (msg == "") != ok {
			t.Errorf("%q: %q", text, msg)
		}
	}
	for r, ok := range map[string]bool{
		"actors.assigned": true, "actors.nobody": false, "core.group:01a186de-611d-7ae4-bda3-1f921281ef39": true,
		"core.person:x": false, "probe.color:01a186de-611d-7ae4-bda3-1f921281ef39": false, "jemand": false,
	} {
		if msg := checkRecipient(probe, r); (msg == "") != ok {
			t.Errorf("%q: %q", r, msg)
		}
	}
	if got := list("a, b;c\nd  e"); strings.Join(got, "|") != "a|b|c|d|e" {
		t.Errorf("list: %v", got)
	}
}

func TestMailer(t *testing.T) {
	srv := smtptest.Start(t)
	m := NewMailer(SMTP{Host: srv.Host, Port: srv.Port, From: "ITSM Prüfung <itsm@example.org>"})
	body := "Zeile mit Umlauten: äöüß\nund einer sehr langen Zeile, " + strings.Repeat("die weitergeht ", 10) + "\n.\nEnde"
	if err := m.Send(t.Context(), Message{To: "anna@example.org", Subject: "Störung: Router ✓", Body: body, Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	got := srv.Mail()
	if len(got) != 1 || got[0].From != "itsm@example.org" || got[0].To[0] != "anna@example.org" {
		t.Fatalf("envelope: %+v", got)
	}
	// SMTP ends the data with a line break.
	if got[0].Subject != "Störung: Router ✓" || strings.TrimSuffix(got[0].Body, "\n") != body {
		t.Fatalf("content: %q / %q", got[0].Subject, got[0].Body)
	}
	if h := got[0].Header; h.Get("Content-Type") != "text/plain; charset=utf-8" || h.Get("Message-Id") == "" || h.Get("Auto-Submitted") != "auto-generated" {
		t.Fatalf("headers: %v", h)
	}
	if err := m.Send(t.Context(), Message{To: "kein Empfänger", Subject: "x", Body: "x"}); err == nil {
		t.Fatal("a wrong address was sent")
	}
	if err := NewMailer(SMTP{Host: "127.0.0.1", Port: 1, From: "a@b.c"}).Send(t.Context(), Message{To: "a@b.c", Subject: "x"}); err == nil {
		t.Fatal("no server, no error")
	}
}

// A notification from before WF19 has no ways yet and goes both; one that names them keeps them.
func TestWaysOfANotification(t *testing.T) {
	base := map[string]any{"id": "0b6f1d3e-2a4c-4e8f-9a1b-3c5d7e9f1a2b"}
	n, err := fromRecord(base)
	if err != nil || !n.Mail || !n.Inbox {
		t.Fatalf("without ways: %+v %v", n, err)
	}
	base["mail"], base["inbox"] = false, true
	if n, _ := fromRecord(base); n.Mail || !n.Inbox {
		t.Fatalf("inbox only: %+v", n)
	}
	s := &Service{mail: NewMailer(SMTP{})}
	if s.reaches(notification{Mail: true}) {
		t.Fatal("mail only reaches someone without a mail server")
	}
	if !s.reaches(notification{Inbox: true}) {
		t.Fatal("the inbox needs no mail server")
	}
	s.mail = NewMailer(SMTP{Host: "mail.test"})
	if !s.reaches(notification{Mail: true}) {
		t.Fatal("mail with a mail server")
	}
}
