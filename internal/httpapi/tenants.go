// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"errors"
	"net/http"
	"slices"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seclog"
)

// --- evaluations across tenants (PL48, Hauptplan E137, E146) ---

// maxTenantList is the most records a list returns per tenant: there is no paging across
// tenants.
const maxTenantList = 500

// tenantQuery is the body of POST /api/v1/tenants:query.
type tenantQuery struct {
	Type    string        `json:"type"`
	Tenants []string      `json:"tenants"`
	Query   records.Query `json:"query"`
}

// tenantSkip names a tenant that gave no rows and why: unknown (not a tenant the administrator
// may work in) or no_read (no right to read the type there).
type tenantSkip struct {
	Tenant string `json:"tenant"`
	Reason string `json:"reason"`
}

// QueryTenants runs the JSON query of one type in each tenant with the rights the
// administrator has there, in the tenant's database, and puts the rows together with the
// tenant as a column; nothing is copied. Only an account of the installation with the right
// report_tenants named in a role at the root may (your decision of 2026-10-30); each tenant
// evaluated gets an entry in its security log. It reads only.
func (s *server) QueryTenants(w http.ResponseWriter, r *http.Request) {
	a := authz.From(r.Context())
	if !a.ReportsAcrossTenants() {
		writeProblem(w, r, http.StatusForbidden, "forbidden",
			"evaluations across tenants need the right "+authz.ActionReportTenants+" on "+authz.TenantType+" in a role at the root, for an account of the installation", nil)
		return
	}
	var in tenantQuery
	if !decode(w, r, &in) {
		return
	}
	if in.Query.After != "" {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "",
			[]records.Problem{{Field: "query.after", Code: "unsupported", Message: "there is no paging across tenants"}})
		return
	}
	aggregate := len(in.Query.Group) > 0 || len(in.Query.Aggregate) > 0
	if !aggregate && (in.Query.Limit <= 0 || in.Query.Limit > maxTenantList) {
		in.Query.Limit = maxTenantList
	}
	tenants := in.Tenants
	if len(tenants) == 0 {
		tenants = a.Tenants
	}
	rows := []map[string]any{}
	skipped := []tenantSkip{}
	for _, tenant := range tenants {
		at := a.Enter(tenant)
		if !slices.Contains(a.Tenants, tenant) || at.CurrentTenant() != tenant {
			skipped = append(skipped, tenantSkip{Tenant: tenant, Reason: "unknown"})
			continue
		}
		ctx := authz.WithAccess(r.Context(), at)
		page, agg, err := s.o.Records.Query(ctx, in.Type, in.Query)
		if errors.Is(err, records.ErrForbidden) {
			skipped = append(skipped, tenantSkip{Tenant: tenant, Reason: "no_read"})
			continue
		}
		if err != nil {
			writeError(w, r, s.o.Logger, err)
			return
		}
		n := 0
		if agg != nil {
			for _, row := range agg.Rows {
				row["tenant"] = tenant
				rows = append(rows, row)
			}
			n = len(agg.Rows)
		} else {
			for _, rec := range page.Items {
				row := map[string]any(rec)
				row["tenant"] = tenant
				rows = append(rows, row)
			}
			n = len(page.Items)
		}
		s.secRecord(r, seclog.Event{Category: seclog.Data, Kind: "report.tenants", Outcome: seclog.Success, Target: in.Type,
			Tenant: tenant, Details: map[string]any{"query": in.Query, "rows": n, "tenants": tenants}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "skipped": skipped})
}
