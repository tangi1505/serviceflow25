// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import "slices"

// PackageIcons are the symbols a package may name as its icon (UI36, UI plan E48): names of
// lucide symbols that the interface knows (web/src/app/icons.ts keeps the same list). A fixed
// list, so that a package never loads a picture from elsewhere.
var PackageIcons = []string{
	"book-open", "box", "calendar", "chart-column", "clipboard-list", "database", "file-text",
	"flask-conical", "folder", "headset", "inbox", "layout-grid", "network", "package", "server",
	"shield", "triangle-alert", "users", "wrench",
}

// IsPackageIcon says whether name is one of PackageIcons.
func IsPackageIcon(name string) bool { return slices.Contains(PackageIcons, name) }
