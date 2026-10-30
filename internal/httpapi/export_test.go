// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// crlf ends a line of a CSV.
const crlf = "\r\n"

// The Prüfweg of UI5 over HTTP: the export of a filtered list holds exactly its rows and
// columns, with labels, titles of references and typed cells, in CSV, XLSX and ODS; another
// person gets only the rows they may read; every export is in the security event log; more
// rows than streamed at once run as a job whose file only the caller may fetch.
func TestExportOverHTTP(t *testing.T) {
	admin := apiServer(t)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	prefix := "UI5 " + suffix
	parent := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "`+prefix+` Eltern"}`, nil)
	for _, body := range []string{
		`{"title": "` + prefix + ` A", "kind": "fault", "priority": 2, "active": true, "due_on": "2026-11-02", "parent": "` + parent.body["id"].(string) + `"}`,
		`{"title": "` + prefix + ` B", "kind": "wish", "priority": 4.5, "active": false}`,
	} {
		if r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", body, nil); r.code != 201 {
			t.Fatalf("record: %d %s", r.code, r.raw)
		}
	}
	request := func(format, lang string) string {
		return `{"format": "` + format + `", "lang": "` + lang + `", "time_zone": "Europe/Berlin", "sort": "title",
			"where": {"field": "title", "starts": "` + prefix + `"},
			"columns": ["title", "kind", "priority", "active", "due_on", "parent", "state"]}`
	}

	// CSV: header and rows as the list shows them.
	code, hdr, body := raw(t, admin, http.MethodPost, "/api/v1/records/probe.probe:export", []byte(request("csv", "de")), map[string]string{"Content-Type": "application/json"})
	if code != 200 || hdr.Get("ITSM-Rows") != "3" || !strings.Contains(hdr.Get("Content-Disposition"), "probe-probe-") ||
		!strings.HasPrefix(hdr.Get("Content-Type"), "text/csv") {
		t.Fatalf("csv: %d %v %s", code, hdr, body)
	}
	want := "\ufeffTitel;Art;Priorität;Aktiv;Fällig am;Übergeordnet;Status\r\n" +
		prefix + " A;Störung;2;ja;02.11.2026;" + prefix + " Eltern;Neu\r\n" +
		prefix + " B;Wunsch;4,5;nein;;;Neu\r\n" +
		prefix + " Eltern;Frage;3;ja;;;Neu\r\n"
	if string(body) != want {
		t.Fatalf("csv:\n got %q\nwant %q", body, want)
	}
	// English labels and words.
	_, _, body = raw(t, admin, http.MethodPost, "/api/v1/records/probe.probe:export", []byte(request("csv", "en")), map[string]string{"Content-Type": "application/json"})
	if !strings.Contains(string(body), "Title;Kind;Priority") || !strings.Contains(string(body), ";Fault;2;yes;") {
		t.Fatalf("english: %q", body)
	}

	// XLSX: typed cells.
	code, _, body = raw(t, admin, http.MethodPost, "/api/v1/records/probe.probe:export", []byte(request("xlsx", "de")), map[string]string{"Content-Type": "application/json"})
	if code != 200 {
		t.Fatalf("xlsx: %d %s", code, body)
	}
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := f.GetRows("Export")
	if len(rows) != 4 || rows[1][1] != "Störung" || rows[1][4] != "02.11.2026" || rows[1][5] != prefix+" Eltern" {
		t.Fatalf("xlsx rows: %q", rows)
	}
	if v, _ := f.GetCellValue("Export", "C3", excelize.Options{RawCellValue: true}); v != "4.5" {
		t.Fatalf("xlsx number: %q", v)
	}
	_ = f.Close()

	// ODS.
	code, _, body = raw(t, admin, http.MethodPost, "/api/v1/records/probe.probe:export", []byte(request("ods", "de")), map[string]string{"Content-Type": "application/json"})
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if code != 200 || err != nil || z.File[0].Name != "mimetype" {
		t.Fatalf("ods: %d %v", code, err)
	}
	for _, e := range z.File {
		if e.Name == "content.xml" {
			rc, _ := e.Open()
			c, _ := io.ReadAll(rc)
			_ = rc.Close()
			if !strings.Contains(string(c), "<text:p>Störung</text:p>") || !strings.Contains(string(c), `office:value="4.5"`) {
				t.Fatalf("ods content: %s", c)
			}
		}
	}

	// Wrong requests are refused at the right field.
	for _, c := range []struct{ body, field string }{
		{`{"format": "pdf"}`, "format"},
		{`{"format": "csv", "columns": ["nofield"]}`, "columns"},
		{`{"format": "csv", "time_zone": "Mars/Base"}`, "time_zone"},
	} {
		r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe:export", c.body, nil)
		if r.code != 422 || !strings.Contains(r.raw, `"field":"`+c.field+`"`) {
			t.Fatalf("wrong export %s: %d %s", c.body, r.code, r.raw)
		}
	}

	// Another person reads only their own records: their export has only those.
	const role = "ui5_own"
	call(t, admin, http.MethodDelete, "/api/v1/roles/"+role, "", nil)
	call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "UI5"}}`, nil)
	for _, p := range []string{`{"type_name": "probe.probe", "action": "read", "scope": "own"}`, `{"type_name": "probe.probe", "action": "create", "scope": "all"}`} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	user := timelineLogin(t, admin, "apitest_ui5", role, `{"last_name": "UI5"}`)
	if r := call(t, user.api, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "`+prefix+` Eigen"}`, nil); r.code != 201 {
		t.Fatalf("own record: %d %s", r.code, r.raw)
	}
	code, hdr, body = raw(t, user.api, http.MethodPost, "/api/v1/records/probe.probe:export", []byte(request("csv", "de")), map[string]string{"Content-Type": "application/json"})
	if code != 200 || hdr.Get("ITSM-Rows") != "1" || !strings.Contains(string(body), prefix+" Eigen") || strings.Contains(string(body), prefix+" A") {
		t.Fatalf("export of the other person: %d %s", code, body)
	}

	// Every export is in the security event log, with type, format and rows.
	r := call(t, admin, http.MethodGet, "/api/v1/audit/data?kind=export&username=apitest_ui5", "", nil)
	if r.code != 200 || !strings.Contains(r.raw, `"target":"probe.probe"`) || !strings.Contains(r.raw, `"rows":1`) || !strings.Contains(r.raw, `"category":"data"`) {
		t.Fatalf("audit: %d %s", r.code, r.raw)
	}

	// More rows than streamed at once: a job writes the file, which only the caller fetches.
	admin.export.MaxDirect = 2
	defer func() { admin.export.MaxDirect = 100000 }()
	code, hdr, body = raw(t, admin, http.MethodPost, "/api/v1/records/probe.probe:export", []byte(request("csv", "de")), map[string]string{"Content-Type": "application/json"})
	if code != 202 || hdr.Get("Location") == "" {
		t.Fatalf("large export: %d %s", code, body)
	}
	var out struct {
		Rows int    `json:"rows"`
		File string `json:"file"`
		Name string `json:"name"`
	}
	deadline := time.Now().Add(time.Minute)
	for out.File == "" {
		if time.Now().After(deadline) {
			t.Fatal("the export job did not finish")
		}
		time.Sleep(200 * time.Millisecond)
		jr := call(t, admin, http.MethodGet, hdr.Get("Location"), "", nil)
		if jr.body["state"] == "discarded" || jr.body["state"] == "cancelled" {
			t.Fatalf("job: %s", jr.raw)
		}
		if jr.body["state"] == "completed" {
			raw, _ := json.Marshal(jr.body["output"])
			_ = json.Unmarshal(raw, &out)
		}
	}
	// The administrator reads the other person's record too: four rows now.
	if out.Rows != 4 || !strings.HasSuffix(out.Name, ".csv") {
		t.Fatalf("job output: %+v", out)
	}
	code, _, body = raw(t, admin, http.MethodGet, "/api/v1/files/"+out.File, nil, nil)
	eltern := prefix + " Eltern;Frage;3;ja;;;Neu" + crlf
	if code != 200 || !strings.HasPrefix(string(body), strings.TrimSuffix(want, eltern)) ||
		!strings.Contains(string(body), prefix+" Eigen;Frage;3;ja;;;Neu"+crlf) || strings.Count(string(body), crlf) != 5 {
		t.Fatalf("file of the job: %d %q", code, body)
	}
	if code, _, _ := raw(t, user.api, http.MethodGet, "/api/v1/files/"+out.File, nil, nil); code != 404 {
		t.Fatalf("another person fetches the file: %d", code)
	}
}
