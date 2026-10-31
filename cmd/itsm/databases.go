// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"context"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/platform"
)

// openDatabases opens the control database and, with ITSM_TENANT_MODE=database, the database
// of every tenant (PL47d, Hauptplan E144), and routes between them by the tenant the request
// works in. A tenant without its URL, or a database behind the migrations, stops it.
func openDatabases(ctx context.Context, cfg platform.Config) (*db.Router, func(), error) {
	control, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	if !cfg.SeparateDatabases() {
		return db.Single(control), control.Close, nil
	}
	tenants, err := db.OpenTenants(ctx, control, cfg.TenantURL)
	if err != nil {
		control.Close()
		return nil, nil, err
	}
	closeAll := func() {
		for _, p := range tenants {
			p.Close()
		}
		control.Close()
	}
	return db.NewRouter(control, tenants, tenantOfRequest), closeAll, nil
}

// tenantOfRequest is the tenant the session of a request works in.
func tenantOfRequest(ctx context.Context) string { return authz.From(ctx).CurrentTenant() }
