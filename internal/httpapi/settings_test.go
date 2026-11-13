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
	"net/http"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
	"group24.de/itsm/internal/sla"
)

// The Prüfweg of SD20 (Hauptplan E180): the packages declare their settings, the server takes
// only declared keys with values that pass, all changes of a node at once or none; the
// Dienststelle Nord gets its own priority matrix and its own time to close, Süd keeps the
// defaults of the package.
func TestSettingsOfTheTreeOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd20"))
	api := apiServer(t)
	ctx := t.Context()
	recs := records.New(api.rt)
	starter := seed.New(api.rt, recs, api.rules)
	for _, dir := range []string{"../../packages/sd", "../../packages/incident"} {
		p := installPackage(t, ctx, api.rt, dir)
		if _, err := starter.Package(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	api.sla.Wrote(ctx, sla.TypeSLA)

	// The declarations: the core first, then the packages by name.
	r := call(t, api, http.MethodGet, "/api/v1/settings/definitions", "", nil)
	var defs []meta.Setting
	if err := json.Unmarshal([]byte(r.raw), &defs); err != nil || r.code != 200 || len(defs) < 30 {
		t.Fatalf("definitions: %d %s", r.code, r.raw)
	}
	byKey := map[string]meta.Setting{}
	for _, d := range defs {
		byKey[d.Key] = d
	}
	if defs[0].Package != "core" || defs[0].Key != "ui.language" || byKey["incident.priority_matrix"].Kind != meta.SettingMatrix ||
		byKey["incident.priority_matrix"].Rows == nil || byKey["incident.close_after"].Kind != meta.SettingDuration ||
		!byKey["security.local_login"].Root || byKey["files.allowed_extensions"].Package != "core" {
		t.Fatalf("definitions: %s", r.raw)
	}

	for _, name := range []string{"sd20_nord", "sd20_sued"} {
		if r := call(t, api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "`+name+`"}`, nil); r.code != 201 {
			t.Fatalf("node %s: %d %s", name, r.code, r.raw)
		}
	}
	const nord, sued = "/api/v1/org/nodes/root.sd20_nord/settings", "/api/v1/org/nodes/root.sd20_sued/settings"

	// One key at a time: unknown keys, values that do not pass, root-only keys below the root.
	for _, c := range []struct{ key, value, code string }{
		{"apitest.unknown", `1`, "unknown"},
		{"probe.priority", `9`, "invalid"},
		{"probe.priority", `"drei"`, "invalid"},
		{"probe.root_only", `1`, "root"},
		{"security.local_login", `"all"`, "root"},
		{"ui.language", `"fr"`, "invalid"},
		{"ui.time_zone", `"Mars/Olympus"`, "invalid"},
		{"files.allowed_extensions", `["pdf", ".zip"]`, "invalid"},
		{"incident.close_after", `-5`, "invalid"},
		{"incident.close_after", `1.5`, "invalid"},
		{"incident.priority_matrix", `{"high": {"high": 1, "medium": 1, "low": 2}}`, "invalid"},
		{"incident.priority_matrix", `{"high": {"high": 1, "medium": 1, "low": 7}, "medium": {"high": 2, "medium": 3, "low": 4}, "low": {"high": 3, "medium": 4, "low": 4}}`, "invalid"},
		{"calendar.default", `"kein Kalender"`, "invalid"},
	} {
		r := call(t, api, http.MethodPut, nord+"/"+c.key, c.value, nil)
		if r.code != 422 || !strings.Contains(r.raw, `"code":"`+c.code+`"`) {
			t.Fatalf("%s = %s: %d %s", c.key, c.value, r.code, r.raw)
		}
	}
	if r := call(t, api, http.MethodPut, "/api/v1/org/nodes/root/settings/probe.root_only", `1`, nil); r.code != 204 {
		t.Fatalf("root only at the root: %d %s", r.code, r.raw)
	}

	// All changes of a node at once: one bad value, nothing is written.
	matrix := `{"high": {"high": 1, "medium": 1, "low": 2}, "medium": {"high": 2, "medium": 3, "low": 4}, "low": {"high": 3, "medium": 4, "low": 4}}`
	if r := call(t, api, http.MethodPatch, nord, `{"incident.priority_matrix": `+matrix+`, "incident.close_after": 0, "probe.priority": "x"}`, nil); r.code != 422 ||
		!strings.Contains(r.raw, "probe.priority") || strings.Contains(r.raw, "incident.priority_matrix") {
		t.Fatalf("patch with a bad value: %d %s", r.code, r.raw)
	}
	own := func(path string) map[string]string {
		t.Helper()
		r := call(t, api, http.MethodGet, path, "", nil)
		var list []struct {
			Key    string          `json:"key"`
			Value  json.RawMessage `json:"value"`
			Source string          `json:"source"`
		}
		if err := json.Unmarshal([]byte(r.raw), &list); err != nil || r.code != 200 {
			t.Fatalf("settings: %d %s", r.code, r.raw)
		}
		out := map[string]string{}
		for _, s := range list {
			if strings.HasPrefix(path, "/api/v1/org/nodes/"+s.Source+"/") {
				out[s.Key] = string(s.Value)
			}
		}
		return out
	}
	if got := own(nord); len(got) != 0 {
		t.Fatalf("nothing written after the bad patch: %v", got)
	}
	if r := call(t, api, http.MethodPatch, nord, `{"incident.priority_matrix": `+matrix+`, "incident.close_after": 0, "probe.priority": 2, "apitest.gone": null}`, nil); r.code != 204 {
		t.Fatalf("patch: %d %s", r.code, r.raw)
	}
	if got := own(nord); len(got) != 3 || got["incident.close_after"] != "0" || got["probe.priority"] != "2" {
		t.Fatalf("own settings of Nord: %v", got)
	}
	if r := call(t, api, http.MethodPatch, nord, `{"probe.priority": null}`, nil); r.code != 204 {
		t.Fatalf("patch removing: %d %s", r.code, r.raw)
	}
	if got := own(nord); len(got) != 2 || got["probe.priority"] != "" {
		t.Fatalf("after removing: %v", got)
	}
	// Removing an own value works for every key; a node that is not there is 404.
	if r := call(t, api, http.MethodPatch, "/api/v1/org/nodes/root.sd20_nope/settings", `{"probe.priority": 1}`, nil); r.code != 404 {
		t.Fatalf("patch at a missing node: %d %s", r.code, r.raw)
	}

	// The priority: high impact and medium urgency are P1 in Nord, P2 in Süd (the default).
	newIncident := func(node, impact, urgency string) map[string]any {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/incident.incident",
			`{"title": "SD20 `+node+`", "impact": "`+impact+`", "urgency": "`+urgency+`", "org_path": "root.`+node+`"}`, nil)
		if r.code != 201 {
			t.Fatalf("incident in %s: %d %s", node, r.code, r.raw)
		}
		return r.body
	}
	inNord := newIncident("sd20_nord", "high", "medium")
	inSued := newIncident("sd20_sued", "high", "medium")
	if inNord["priority"] != 1.0 || inSued["priority"] != 2.0 {
		t.Fatalf("priority Nord %v, Süd %v", inNord["priority"], inSued["priority"])
	}
	// Süd sets the root's matrix and gets it; without it again the package's.
	if r := call(t, api, http.MethodPut, "/api/v1/org/nodes/root/settings/incident.priority_matrix",
		`{"high": {"high": 1, "medium": 3, "low": 3}, "medium": {"high": 3, "medium": 3, "low": 4}, "low": {"high": 3, "medium": 4, "low": 4}}`, nil); r.code != 204 {
		t.Fatalf("matrix at the root: %d %s", r.code, r.raw)
	}
	if got := newIncident("sd20_sued", "high", "medium")["priority"]; got != 3.0 {
		t.Fatalf("Süd inherits the root's matrix: %v", got)
	}
	if got := newIncident("sd20_nord", "high", "medium")["priority"]; got != 1.0 {
		t.Fatalf("Nord keeps its own: %v", got)
	}
	if r := call(t, api, http.MethodDelete, "/api/v1/org/nodes/root/settings/incident.priority_matrix", "", nil); r.code != 204 {
		t.Fatalf("unset at the root: %d %s", r.code, r.raw)
	}
	// An open incident computes it anew when it is saved next.
	if r := call(t, api, http.MethodPatch, nord, `{"incident.priority_matrix": null}`, nil); r.code != 204 {
		t.Fatalf("unset in Nord: %d %s", r.code, r.raw)
	}
	id := inNord["id"].(string)
	r = call(t, api, http.MethodPatch, "/api/v1/records/incident.incident/"+id, `{"title": "SD20 Nord neu"}`,
		map[string]string{"If-Match": fmt.Sprintf(`"%v"`, inNord["version"])})
	if r.code != 200 || r.body["priority"] != 2.0 {
		t.Fatalf("Nord without its matrix: %d %s", r.code, r.raw)
	}

	// Closing: Nord 0 means no clock; Süd 3300 minutes as the package says; Nord 660 minutes.
	fixed := ""
	for _, it := range call(t, api, http.MethodGet, "/api/v1/records/sd.solution_kind?limit=50", "", nil).body["items"].([]any) {
		if k := it.(map[string]any); k["key"] == "fixed" {
			fixed = k["id"].(string)
		}
	}
	var closeSLA string
	if err := api.pool.QueryRow(ctx, "SELECT ref FROM package_seed WHERE org_path = 'root' AND package = 'incident' AND key = 'sla_close'").Scan(&closeSLA); err != nil {
		t.Fatal(err)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/records/core.sla/"+closeSLA, "", nil); r.body["target_setting"] != "incident.close_after" {
		t.Fatalf("the SLA of the start data names the setting: %s", r.raw)
	}
	resolve := func(rec map[string]any) {
		t.Helper()
		id := rec["id"].(string)
		cur := call(t, api, http.MethodGet, "/api/v1/records/incident.incident/"+id, "", nil)
		r := call(t, api, http.MethodPost, "/api/v1/records/incident.incident/"+id+":transition",
			`{"transition": "resolve", "fields": {"solution": "SD20", "solution_kind": "`+fixed+`"}}`,
			map[string]string{"If-Match": fmt.Sprintf(`"%v"`, cur.body["version"])})
		if r.code != 200 {
			t.Fatalf("resolve: %d %s", r.code, r.raw)
		}
	}
	target := func(rec map[string]any) int64 {
		t.Helper()
		var seconds int64
		err := api.pool.QueryRow(ctx, "SELECT target_seconds FROM clock WHERE record_id = $1 AND sla = $2", rec["id"], closeSLA).Scan(&seconds)
		if err != nil {
			return -1
		}
		return seconds
	}
	resolve(inNord)
	resolve(inSued)
	if got := target(inNord); got != -1 {
		t.Fatalf("Nord with 0 has a clock to close: %d", got)
	}
	if got := target(inSued); got != 3300*60 {
		t.Fatalf("Süd closes after %d s", got)
	}
	if r := call(t, api, http.MethodPut, nord+"/incident.close_after", `660`, nil); r.code != 204 {
		t.Fatalf("close after 660 in Nord: %d %s", r.code, r.raw)
	}
	later := newIncident("sd20_nord", "low", "low")
	resolve(later)
	if got := target(later); got != 660*60 {
		t.Fatalf("Nord closes after %d s", got)
	}

	// An SLA may only name a declared setting of minutes.
	for _, c := range []struct{ key, want string }{{"probe.tenant", "not minutes"}, {"nope.none", "no package declares"}} {
		r := call(t, api, http.MethodPost, "/api/v1/records/core.sla", `{"name": "SD20 `+c.key+`", "target_type": "incident.incident",
			"target_minutes": 60, "stop": "false", "target_setting": "`+c.key+`"}`, nil)
		if r.code != 422 || !strings.Contains(r.raw, c.want) {
			t.Fatalf("SLA with %s: %d %s", c.key, r.code, r.raw)
		}
	}

	// A package whose formula reads a setting nobody declares is refused.
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":   {Data: []byte("name: sd20probe\nversion: 0.1.0\nlabel: {de: P, en: P}\n")},
		"types/doc.yaml": {Data: []byte("name: doc\nlabel: {de: D, en: D}\nfields:\n  - {name: n, type: number, label: {de: N, en: N}, formula: 'setting(\"sd20probe.missing\")'}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, api.rt.Control(), p); err == nil || !strings.Contains(err.Error(), "no package declares") {
		t.Fatalf("formula with an undeclared setting: %v", err)
	}
}
