// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"os"
	"sync"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
)

// The mirror of an administrator of the installation (PL47b) with requests at once right after
// switching into a tenant with a database of its own, as the browser sends them: every request
// goes through, the mirror is there once (Nachtrag 2026-11-03, found in the test of PL49).
func TestMirrorAtOnceOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_mirror_ctl"))
	urls := map[string]string{"root.ta": dbtest.Isolated(t, base, "itsmtest_api_mirror_ta"), "root.tb": dbtest.Isolated(t, base, "itsmtest_api_mirror_tb")}
	admin := apiServerWith(t, urls)
	tenantsInDatabases(t, admin, urls)

	for round, path := range []string{"root.ta", "root.tb", "root.ta"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/auth/context", `{"tenant": "`+path+`"}`, nil); r.code != 200 {
			t.Fatalf("switch to %s: %d %s", path, r.code, r.raw)
		}
		var wg sync.WaitGroup
		for i := range 30 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe?limit=1", "", nil); r.code != 200 {
					t.Errorf("round %d, request %d in %s: %d %s", round, i, path, r.code, r.raw)
				}
			}()
		}
		wg.Wait()
	}
	var n int
	if err := admin.rt.For("root.ta").QueryRow(t.Context(), "SELECT count(*) FROM account WHERE person_id = $1", admin.person).Scan(&n); err != nil || n != 1 {
		t.Fatalf("mirror accounts in A: %d %v", n, err)
	}
}
