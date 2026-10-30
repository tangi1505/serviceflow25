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
	"strings"
	"testing"
)

// The Prüfweg of UI7 over HTTP: an image pasted into an entry becomes a file at the record
// and stays in the text; the HTML is sanitized; the image of a private entry is as private as
// the entry (documents, download, history); a requester with comment, without update, may
// add an image to a public entry; someone else's upload is refused; a rich field is
// sanitized, and lists and exports get its plain text.
func TestRichTextOverHTTP(t *testing.T) {
	admin := apiServer(t)
	timelineRoles(t, admin, "ui7")
	requester := timelineLogin(t, admin, "apitest_ui7_req", "ui7_requester", `{"last_name": "UI7 Anforderer"}`)
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "UI7 Bild"}`, nil)
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)
	upload := func(api *testAPI, name string) string {
		t.Helper()
		code, _, body := raw(t, api, http.MethodPost, "/api/v1/files?name="+name, []byte("\x89PNG\r\n\x1a\nbild"), map[string]string{"Content-Type": "application/octet-stream"})
		if code != 201 {
			t.Fatalf("upload: %d %s", code, body)
		}
		var f struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &f)
		return f.ID
	}
	documents := func(api *testAPI) string {
		t.Helper()
		r := call(t, api, http.MethodGet, base+"/documents", "", nil)
		if r.code != 200 {
			t.Fatalf("documents: %d %s", r.code, r.raw)
		}
		return r.raw
	}

	// A private entry with an image and something that must not run.
	secret := upload(admin, "intern.png")
	r := call(t, admin, http.MethodPost, base+"/timeline", `{"format": "html", "private": true,
		"body": "<p onclick=\"x()\">Siehe <strong>Bild</strong><script>alert(1)</script></p><img src=\"/api/v1/files/`+secret+`\" alt=\"Router\">"}`, nil)
	if r.code != 201 || r.body["format"] != "html" ||
		r.body["body"] != `<p>Siehe <strong>Bild</strong></p><img src="/api/v1/files/`+secret+`" alt="Router">` {
		t.Fatalf("private entry: %d %s", r.code, r.raw)
	}
	if !strings.Contains(documents(admin), secret) {
		t.Fatal("the image is not a file at the record")
	}
	if strings.Contains(documents(requester.api), secret) {
		t.Fatal("the image of a private entry shows in the documents of a reader without read_private")
	}
	if code, _, _ := raw(t, requester.api, http.MethodGet, "/api/v1/files/"+secret, nil, nil); code != 404 {
		t.Fatalf("download of the private image: %d", code)
	}
	if code, _, _ := raw(t, admin, http.MethodGet, "/api/v1/files/"+secret, nil, nil); code != 200 {
		t.Fatalf("download by the administrator: %d", code)
	}
	if h := call(t, admin, http.MethodGet, base+"/history", "", nil); strings.Contains(h.raw, secret) {
		t.Fatalf("the history names the private image: %s", h.raw)
	}

	// The requester answers with an image of their own, without update on the record.
	own := upload(requester.api, "foto.png")
	r = call(t, requester.api, http.MethodPost, base+"/timeline", `{"format": "html", "body": "<p>Hier:</p><img src=\"/api/v1/files/`+own+`\">"}`, nil)
	if r.code != 201 {
		t.Fatalf("requester's image: %d %s", r.code, r.raw)
	}
	if !strings.Contains(documents(requester.api), own) {
		t.Fatal("the public image is not in the documents")
	}
	if h := call(t, admin, http.MethodGet, base+"/history", "", nil); !strings.Contains(h.raw, own) {
		t.Fatalf("the history lacks the public image: %s", h.raw)
	}
	// Someone else's upload cannot be shown.
	other := upload(admin, "fremd.png")
	r = call(t, requester.api, http.MethodPost, base+"/timeline", `{"format": "html", "body": "<img src=\"/api/v1/files/`+other+`\">"}`, nil)
	if r.code != 422 || !strings.Contains(r.raw, `"code":"image"`) {
		t.Fatalf("someone else's upload: %d %s", r.code, r.raw)
	}
	// Empty paragraphs are no entry; a wrong format is refused.
	for body, code := range map[string]string{`{"format": "html", "body": "<p></p><p> </p>"}`: "required", `{"format": "md", "body": "x"}`: "choice"} {
		if r := call(t, admin, http.MethodPost, base+"/timeline", body, nil); r.code != 422 || !strings.Contains(r.raw, `"code":"`+code+`"`) {
			t.Fatalf("%s: %d %s", body, r.code, r.raw)
		}
	}

	// A rich field is sanitized; the export gets its plain text.
	r = call(t, admin, http.MethodPatch, base, `{"solution": "<p>Kabel <em>getauscht</em></p><script>x</script>"}`, map[string]string{"If-Match": `"1"`})
	if r.code != 200 || r.body["solution"] != "<p>Kabel <em>getauscht</em></p>" {
		t.Fatalf("rich field: %d %s", r.code, r.raw)
	}
	code, _, body := raw(t, admin, http.MethodPost, "/api/v1/records/probe.probe:export",
		[]byte(`{"format": "csv", "columns": ["title", "solution"], "where": {"field": "id", "eq": "`+rec.body["id"].(string)+`"}}`),
		map[string]string{"Content-Type": "application/json"})
	if code != 200 || !strings.Contains(string(body), "UI7 Bild;Kabel getauscht") {
		t.Fatalf("export of a rich field: %d %q", code, body)
	}
	// The type says which text fields are rich.
	if r := call(t, admin, http.MethodGet, "/api/v1/types/probe.probe", "", nil); !strings.Contains(r.raw, `"name":"solution","type":"text","label":{"de":"Lösung","en":"Solution"},"max_length":2000,"format":"rich"`) {
		t.Fatalf("type: %s", r.raw)
	}
}
