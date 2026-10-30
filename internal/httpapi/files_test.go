// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/files"
	"group24.de/itsm/internal/records"
)

func testFiles(t *testing.T, pool *pgxpool.Pool, recs *records.Service) *files.Service {
	t.Helper()
	store, err := files.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return files.New(pool, store, recs, nil, quiet)
}

// raw sends a request with the session and the CSRF header and returns status, headers and
// body as they are (call decodes JSON).
func raw(t *testing.T, api *testAPI, method, path string, body []byte, hdr map[string]string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, api.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(csrfHeader, csrfValue)
	if api.cookie != "" {
		req.Header.Set("Cookie", sessionCookie+"="+api.cookie)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := api.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, b
}

// The acceptance of PL18 over HTTP: upload a file, attach it to a record, download it with
// the same SHA-256; rights decide who downloads; blocked extensions and types without the
// trait documents are refused.
func TestFilesOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	content := bytes.Repeat([]byte("Logzeile mit Umlauten: äöü\n"), 20000)
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])

	code, hdr, body := raw(t, admin, http.MethodPost, "/api/v1/files?name=server.log", content, map[string]string{"Content-Type": "application/octet-stream"})
	var up map[string]any
	_ = json.Unmarshal(body, &up)
	if code != 201 || up["sha256"] != want || up["state"] != "temporary" || !strings.HasPrefix(hdr.Get("Location"), "/api/v1/files/") {
		t.Fatalf("upload: %d %s", code, body)
	}
	fileID := up["id"].(string)
	// The name may also come from Content-Disposition.
	code, _, body = raw(t, admin, http.MethodPost, "/api/v1/files", []byte("x"), map[string]string{"Content-Disposition": `attachment; filename="notiz.txt"`})
	if code != 201 || !strings.Contains(string(body), `"name":"notiz.txt"`) {
		t.Fatalf("upload with Content-Disposition: %d %s", code, body)
	}
	if code, _, body = raw(t, admin, http.MethodPost, "/api/v1/files?name=setup.exe", []byte("MZ"), nil); code != 422 || !strings.Contains(string(body), "blocked") {
		t.Fatalf("blocked extension: %d %s", code, body)
	}

	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "with a file"}`, nil)
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)
	if r := call(t, admin, http.MethodPost, base+"/documents", `{"file": "`+fileID+`"}`, nil); r.code != 201 || r.body["state"] != "stored" {
		t.Fatalf("attach: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"/documents", "", nil); r.code != 200 || !strings.Contains(r.raw, fileID) {
		t.Fatalf("documents: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"/history", "", nil); !strings.Contains(r.raw, `"action":"attach"`) {
		t.Fatalf("history: %s", r.raw)
	}

	code, hdr, body = raw(t, admin, http.MethodGet, "/api/v1/files/"+fileID, nil, nil)
	got := sha256.Sum256(body)
	if code != 200 || hex.EncodeToString(got[:]) != want || hdr.Get("ITSM-SHA256") != want ||
		!strings.HasPrefix(hdr.Get("Content-Disposition"), `attachment; filename="server.log"`) || hdr.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("download: %d %v", code, hdr)
	}
	// Ranges work.
	if code, _, body = raw(t, admin, http.MethodGet, "/api/v1/files/"+fileID, nil, map[string]string{"Range": "bytes=0-3"}); code != 206 || string(body) != "Logz" {
		t.Fatalf("range: %d %q", code, body)
	}

	// A person without rights on probe.probe gets 404 for the file and may not attach.
	acc, err := admin.auth.CreateAccount(ctx, "apitest_files", "Files", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "Files"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_files", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	other := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	if code, _, _ := raw(t, other, http.MethodGet, "/api/v1/files/"+fileID, nil, nil); code != 404 {
		t.Fatalf("download without rights: %d", code)
	}
	// Their own upload they may read while it is temporary, but not attach to a foreign record.
	code, _, body = raw(t, other, http.MethodPost, "/api/v1/files?name=own.txt", []byte("own"), nil)
	var own map[string]any
	_ = json.Unmarshal(body, &own)
	if code != 201 {
		t.Fatalf("own upload: %d %s", code, body)
	}
	if code, _, body = raw(t, other, http.MethodGet, "/api/v1/files/"+own["id"].(string), nil, nil); code != 200 || string(body) != "own" {
		t.Fatalf("own download: %d %s", code, body)
	}
	// A temporary file belongs to its uploader alone, the administration included (E95).
	if code, _, _ = raw(t, admin, http.MethodGet, "/api/v1/files/"+own["id"].(string), nil, nil); code != 404 {
		t.Fatalf("admin reads a foreign temporary file: %d", code)
	}
	if r := call(t, other, http.MethodPost, base+"/documents", `{"file": "`+own["id"].(string)+`"}`, nil); r.code != 404 {
		t.Fatalf("attach to an unreadable record: %d %s", r.code, r.raw)
	}

	// A type without the trait documents: 404 on the documents.
	person := "/api/v1/records/core.person/" + p.body["id"].(string)
	if r := call(t, admin, http.MethodGet, person+"/documents", "", nil); r.code != 404 || !strings.Contains(r.raw, "documents") {
		t.Fatalf("documents of a type without the trait: %d %s", r.code, r.raw)
	}

	// Detach, then the file is no longer listed and no longer readable through the record.
	if code, _, body = raw(t, admin, http.MethodDelete, base+"/documents/"+fileID, nil, nil); code != 204 {
		t.Fatalf("detach: %d %s", code, body)
	}
	if r := call(t, admin, http.MethodGet, base+"/documents", "", nil); r.code != 200 || strings.Contains(r.raw, fileID) {
		t.Fatalf("documents after detach: %d %s", r.code, r.raw)
	}
	r := call(t, admin, http.MethodGet, "/api/v1/openapi.json", "", nil)
	paths, _ := r.body["paths"].(map[string]any)
	if paths["/api/v1/records/probe.probe/{id}/documents/{fileId}"] == nil || paths["/api/v1/files"] == nil {
		t.Fatal("file paths missing in the OpenAPI")
	}
}
