// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/rules"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD16a (SD-Plan E8, N12): persons without any grant on articles read the
// published ones for everyone at their node and above, those shared with a group of theirs
// wherever they lie, and neither drafts nor those only for agents; the search keeps to the
// same; nobody changes an article by reading it.
func TestKnowledgeOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd16a"))
	admin := apiServer(t)
	for _, dir := range []string{"../../packages/sd", "../../packages/knowledge"} {
		installPackage(t, t.Context(), admin.rt, dir)
	}
	const arts = "/api/v1/records/knowledge.article"

	// The type offers the area "Wissen" in the portal and brings its readers.
	ty := call(t, admin, http.MethodGet, "/api/v1/types/knowledge.article", "", nil)
	if ty.code != 200 || !strings.Contains(ty.raw, `"readers"`) || !strings.Contains(ty.raw, `"group":"category"`) {
		t.Fatalf("type: %d %.600s", ty.code, ty.raw)
	}

	for _, n := range []string{"nord", "sued"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "`+n+`"}`, nil); r.code != 201 {
			t.Fatalf("node %s: %d %s", n, r.code, r.raw)
		}
	}
	// A role without any permission: what these persons read, the readers give.
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "sd16_user", "label": {"de": "SD16"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	nord := timelineLogin(t, admin, "apitest_sd16_nord", "sd16_user", `{"last_name": "Nord", "org_path": "root.nord"}`)
	// Since SD17 an article is published once an editor approved it.
	editor := timelineLogin(t, admin, "apitest_sd16_editor", "sd16_user", `{"last_name": "Redaktion"}`)
	sued := timelineLogin(t, admin, "apitest_sd16_sued", "sd16_user", `{"last_name": "Süd", "org_path": "root.sued"}`)
	g := call(t, admin, http.MethodPost, "/api/v1/records/core.group", `{"name": "Netz SD16"}`, nil)
	if g.code != 201 {
		t.Fatalf("group: %d %s", g.code, g.raw)
	}
	group := g.body["id"].(string)
	if r := call(t, admin, http.MethodPost, "/api/v1/records/core.group_member", `{"group": "`+group+`", "person": "`+sued.person+`"}`, nil); r.code != 201 {
		t.Fatalf("member: %d %s", r.code, r.raw)
	}

	article := func(title, audience, node string, publish bool, extra string) string {
		t.Helper()
		body := `{"title": "` + title + `", "body": "<p>` + title + `: VPN neu starten.</p>", "audience": "` + audience + `"`
		if node != "" {
			body += `, "org_path": "` + node + `"`
		}
		r := call(t, admin, http.MethodPost, arts, body+extra+`}`, nil)
		if r.code != 201 || r.body["state"] != "draft" {
			t.Fatalf("article %s: %d %s", title, r.code, r.raw)
		}
		id := r.body["id"].(string)
		if publish {
			publishArticle(t, admin, admin, editor, arts, id)
		}
		return id
	}
	forAll := article("Für alle", "all", "", true, "")
	article("Nur Nord", "all", "root.nord", true, "")
	article("Entwurf", "all", "", false, "")
	agents := article("Nur Bearbeiter", "agents", "", true, "")
	article("Gruppe Netz", "groups", "root.nord", true, `, "actors": {"shared": [{"type": "core.group", "id": "`+group+`"}]}`)

	titles := func(api *testAPI) []string {
		t.Helper()
		r := call(t, api, http.MethodGet, arts+"?limit=50", "", nil)
		if r.code != 200 {
			t.Fatalf("articles: %d %s", r.code, r.raw)
		}
		var out []string
		for _, it := range r.body["items"].([]any) {
			out = append(out, it.(map[string]any)["title"].(string))
		}
		slices.Sort(out)
		return out
	}
	if got := titles(nord.api); !slices.Equal(got, []string{"Für alle", "Nur Nord"}) {
		t.Fatalf("Nord reads %v", got)
	}
	if got := titles(sued.api); !slices.Equal(got, []string{"Für alle", "Gruppe Netz"}) {
		t.Fatalf("Süd, member of the group, reads %v", got)
	}
	if got := titles(admin); len(got) != 5 {
		t.Fatalf("the administrator reads %v", got)
	}
	// One by one the same; reading is not changing.
	if r := call(t, nord.api, http.MethodGet, arts+"/"+agents, "", nil); r.code != 404 {
		t.Fatalf("Nord reads the article for agents: %d %s", r.code, r.raw)
	}
	if r := call(t, nord.api, http.MethodGet, arts+"/"+forAll, "", nil); r.code != 200 || !strings.Contains(r.raw, "VPN neu starten") {
		t.Fatalf("Nord reads the article for all: %d %s", r.code, r.raw)
	}
	if r := call(t, nord.api, http.MethodPatch, arts+"/"+forAll, `{"title": "geändert"}`, map[string]string{"If-Match": `"1"`}); r.code != 403 && r.code != 404 {
		t.Fatalf("Nord changes the article: %d %s", r.code, r.raw)
	}

	// An English title (SD16c): read in English where there is one, else German.
	// Since SD17 the title of a published article changes once the editor approved it.
	if r := call(t, admin, http.MethodPatch, arts+"/"+forAll, `{"title": {"en": "For everyone"}}`, map[string]string{"If-Match": versionOf(t, admin, arts+"/"+forAll)}); r.code != 200 {
		t.Fatalf("English title: %d %s", r.code, r.raw)
	}
	approveOpen(t, editor, arts, forAll)
	if r := call(t, nord.api, http.MethodGet, arts+"/"+forAll, "", map[string]string{"Accept-Language": "en"}); r.code != 200 || r.body["title"] != "For everyone" || !strings.Contains(r.raw, "VPN neu starten") {
		t.Fatalf("Nord reads in English: %d %s", r.code, r.raw)
	}
	if r := call(t, nord.api, http.MethodGet, arts+"/"+forAll, "", map[string]string{"Accept-Language": "de"}); r.body["title"] != "Für alle" {
		t.Fatalf("Nord reads in German: %d %s", r.code, r.raw)
	}

	// The search keeps to the readers: every article says VPN, Süd finds two.
	r := call(t, sued.api, http.MethodGet, "/api/v1/search?q=VPN&types=knowledge.article", "", nil)
	var hits []map[string]any
	if r.code != 200 || json.Unmarshal([]byte(r.raw), &hits) != nil {
		t.Fatalf("search: %d %s", r.code, r.raw)
	}
	found := map[string]bool{}
	for _, h := range hits {
		found[h["title"].(string)] = true
	}
	if got := slices.Sorted(maps.Keys(found)); !slices.Equal(got, []string{"Für alle", "Gruppe Netz"}) {
		t.Fatalf("Süd finds %v", got)
	}
}

// The Prüfweg of SD16d (SD-Plan E11): who creates an article becomes its author, unless the
// request names one; valid until yesterday it is gone from the portal at once; the daily rule
// sets it expired and the author gets a mail, 14 days before a reminder.
func TestKnowledgeLifecycleOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd16d"))
	admin := apiServer(t)
	ctx := t.Context()
	starter := seed.New(admin.rt, records.New(admin.rt), admin.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 28}, {"../../packages/knowledge", 9}} {
		p := installPackage(t, ctx, admin.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	const arts = "/api/v1/records/knowledge.article"
	for _, role := range []string{"sd16d_author", "sd16d_user"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "SD16d"}}`, nil); r.code != 201 {
			t.Fatalf("role: %d %s", r.code, r.raw)
		}
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/sd16d_author/permissions", `{"type_name": "knowledge.article", "action": "*", "scope": "all"}`, nil); r.code != 204 {
		t.Fatalf("permission: %d %s", r.code, r.raw)
	}
	author := timelineLogin(t, admin, "apitest_sd16d_author", "sd16d_author", `{"last_name": "Autorin", "email": "autorin16d@example.org", "language": "de"}`)
	reader := timelineLogin(t, admin, "apitest_sd16d_user", "sd16d_user", `{"last_name": "Leser"}`)
	// Since SD17 an article is published once an editor approved it.
	editor := timelineLogin(t, admin, "apitest_sd16d_editor", "sd16d_user", `{"last_name": "Redaktion"}`)
	authorOf := func(rec map[string]any) string {
		hs, _ := rec["actors"].(map[string]any)["author"].([]any)
		if len(hs) != 1 {
			return fmt.Sprint(hs)
		}
		return fmt.Sprint(hs[0].(map[string]any)["id"])
	}
	article := func(api *testAPI, title, expires, extra string) string {
		t.Helper()
		body := `{"title": "` + title + `", "body": "<p>` + title + `</p>", "audience": "all"`
		if expires != "" {
			body += `, "expires_on": "` + expires + `"`
		}
		r := call(t, api, http.MethodPost, arts, body+extra+`}`, nil)
		if r.code != 201 {
			t.Fatalf("article %s: %d %s", title, r.code, r.raw)
		}
		id := r.body["id"].(string)
		publishArticle(t, api, admin, editor, arts, id)
		return id
	}

	// The author is who creates it; a request that names one keeps it.
	yesterday := article(author.api, "Gestern", rules.Today(-1), "")
	soon := article(author.api, "In 14 Tagen", rules.Today(14), "")
	article(author.api, "Ohne Datum", "", "")
	named := article(admin, "Benannt", "", `, "actors": {"author": [{"type": "core.person", "id": "`+reader.person+`"}]}`)
	if got := authorOf(call(t, admin, http.MethodGet, arts+"/"+yesterday, "", nil).body); got != author.person {
		t.Fatalf("the author of the article is %s, want %s", got, author.person)
	}
	if got := authorOf(call(t, admin, http.MethodGet, arts+"/"+named, "", nil).body); got != reader.person {
		t.Fatalf("the author named by the request is %s, want %s", got, reader.person)
	}

	// Only a role a person may hold takes the one who writes.
	if r := call(t, admin, http.MethodPut, "/api/v1/rules/sd16d_groups", `{"type_name": "knowledge.article", "trigger": "create", "mode": "before",
		"actions": [{"assign": {"role": "shared", "actor": true}}]}`, nil); r.code != 422 || !strings.Contains(r.raw, "a person may hold") {
		t.Fatalf("actor in a role of groups: %d %s", r.code, r.raw)
	}

	// Valid until yesterday: gone for readers before any rule ran, in the list and one by one.
	titles := func() []string {
		t.Helper()
		r := call(t, reader.api, http.MethodGet, arts+"?limit=50", "", nil)
		var out []string
		for _, it := range r.body["items"].([]any) {
			out = append(out, it.(map[string]any)["title"].(string))
		}
		slices.Sort(out)
		return out
	}
	if got := titles(); !slices.Equal(got, []string{"Benannt", "In 14 Tagen", "Ohne Datum"}) {
		t.Fatalf("the reader reads %v", got)
	}
	if r := call(t, reader.api, http.MethodGet, arts+"/"+yesterday, "", nil); r.code != 404 {
		t.Fatalf("the reader reads the expired article: %d %s", r.code, r.raw)
	}

	// The daily rules: the one of yesterday expires, the author hears of it and of the one
	// that expires in 14 days.
	if _, err := admin.rules.RunSchedules(ctx, time.Now().Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if r := call(t, admin, http.MethodGet, arts+"/"+yesterday, "", nil); r.body["state"] != "expired" {
		t.Fatalf("after the daily rule: %v", r.body["state"])
	}
	if r := call(t, admin, http.MethodGet, arts+"/"+soon, "", nil); r.body["state"] != "published" {
		t.Fatalf("the article valid for 14 days: %v", r.body["state"])
	}
	mail := func(subject, body string) bool {
		for _, m := range admin.smtp.Mail() {
			if slices.Contains(m.To, "autorin16d@example.org") && strings.Contains(m.Subject, subject) && strings.Contains(m.Body, body) {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(20 * time.Second)
	for !mail("Ihr Artikel ist abgelaufen: Gestern", "nicht mehr zu sehen") || !mail("In 14 Tagen", "läuft in 14 Tagen ab") {
		for _, d := range admin.rt.Databases() {
			if err := admin.ruleEvents.Dispatch(d.Bind(ctx)); err != nil {
				t.Fatal(err)
			}
			if err := admin.notifyEvents.Dispatch(d.Bind(ctx)); err != nil {
				t.Fatal(err)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the mails to the author: %v", admin.smtp.Mail())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// publishArticle publishes an article as SD17 asks (SD-Plan E16): the editor holds the role
// editors, the article is submitted and the editor approves. Before SD17 the tests took the
// transition "publish" themselves; now it follows the approval only.
func publishArticle(t *testing.T, api, admin *testAPI, editor timelineUser, arts, id string) {
	t.Helper()
	url := arts + "/" + id
	if r := call(t, admin, http.MethodPatch, url, `{"actors": {"editors": [{"type": "core.person", "id": "`+editor.person+`"}]}}`,
		map[string]string{"If-Match": versionOf(t, admin, url)}); r.code != 200 {
		t.Fatalf("editors of %s: %d %s", id, r.code, r.raw)
	}
	if r := call(t, api, http.MethodPost, url+":transition", `{"transition": "submit"}`, nil); r.code != 200 || r.body["state"] != "review" {
		t.Fatalf("submit %s: %d %s", id, r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, url+"/approvals", `{"stages": [{"approvers": [{"type": "core.person", "id": "`+editor.person+`"}]}]}`, nil); r.code != 201 {
		t.Fatalf("ask the editor for %s: %d %s", id, r.code, r.raw)
	}
	approveOpen(t, editor, arts, id)
	if r := call(t, admin, http.MethodGet, url, "", nil); r.body["state"] != "published" {
		t.Fatalf("publish %s: %v", id, r.body["state"])
	}
}

// approveOpen lets the editor approve the open approval of an article.
func approveOpen(t *testing.T, editor timelineUser, arts, id string) {
	t.Helper()
	url := arts + "/" + id
	r := call(t, editor.api, http.MethodGet, url+"/approvals", "", nil)
	var list []map[string]any
	if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil || len(list) == 0 || list[0]["state"] != "open" {
		t.Fatalf("approvals of %s: %d %s", id, r.code, r.raw)
	}
	if r := call(t, editor.api, http.MethodPost, url+"/approvals/"+list[0]["id"].(string)+":decide", `{"decision": "approve"}`, nil); r.code != 200 {
		t.Fatalf("approve %s: %d %s", id, r.code, r.raw)
	}
}

// versionOf is the ETag of a record as If-Match wants it.
func versionOf(t *testing.T, api *testAPI, url string) string {
	t.Helper()
	r := call(t, api, http.MethodGet, url, "", nil)
	if r.code != 200 {
		t.Fatalf("read %s: %d %s", url, r.code, r.raw)
	}
	return fmt.Sprintf(`"%v"`, r.body["version"])
}
