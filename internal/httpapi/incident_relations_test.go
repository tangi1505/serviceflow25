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

	"group24.de/itsm/internal/db/dbtest"
)

// The Prüfweg of SD4 (SD-Plan E7): the kind "betrifft" of the package sd links any type with
// the trait ticket (N11) to a record of any type; both ends list the relation; a record without
// the trait cannot be the ticket.
func TestIncidentAffectsOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd4"))
	api := apiServer(t)
	for _, dir := range []string{"../../packages/sd", "../../packages/incident"} {
		installPackage(t, t.Context(), api.rt, dir)
	}

	kinds := call(t, api, http.MethodGet, "/api/v1/relations/kinds", "", nil)
	if !strings.Contains(kinds.raw, `"name":"affects"`) || !strings.Contains(kinds.raw, `"trait:sd.ticket"`) || !strings.Contains(kinds.raw, `"betroffen von"`) {
		t.Fatalf("kinds: %s", kinds.raw)
	}
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	inc := create("incident.incident", `{"title": "SD4 Fachverfahren", "impact": "high", "urgency": "high"}`)
	probe := create("probe.probe", `{"title": "SD4 Server"}`)
	relate := func(fromType, fromID, toType, toID string) resp {
		return call(t, api, http.MethodPost, "/api/v1/relations",
			fmt.Sprintf(`{"kind": "sd.affects", "from": {"type": %q, "id": %q}, "to": {"type": %q, "id": %q}}`, fromType, fromID, toType, toID), nil)
	}

	// The incident affects the probe: both list it.
	if r := relate("incident.incident", inc, "probe.probe", probe); r.code != 201 {
		t.Fatalf("relate: %d %s", r.code, r.raw)
	}
	for _, end := range []string{"/api/v1/records/incident.incident/" + inc, "/api/v1/records/probe.probe/" + probe} {
		r := call(t, api, http.MethodGet, end+"/relations", "", nil)
		var list []map[string]any
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil || len(list) != 1 || list[0]["kind"] != "sd.affects" {
			t.Fatalf("relations of %s: %d %s", end, r.code, r.raw)
		}
	}
	// A record without the trait is no ticket.
	if r := relate("probe.probe", probe, "incident.incident", inc); r.code != 422 {
		t.Fatalf("a probe affects: %d %s", r.code, r.raw)
	}
}
