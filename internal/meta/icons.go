// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import "slices"

// PackageIcons are the symbols a package may name as its icon (UI36, UI plan E48), a reading
// area of the portal (UI41) and a tile of the portal (UI42, UI plan E53): names of lucide
// symbols that the interface knows (web/src/app/icons.ts keeps the same list, TestIconsOfWeb
// holds them equal). A fixed list, so that nothing loads a picture from elsewhere. Since UI42
// with the topics of a portal: key, monitor, printer, phone, help.
var PackageIcons = []string{
	"book-open", "box", "calendar", "chart-column", "circle-help", "clipboard-list", "database",
	"file-text", "flask-conical", "folder", "headset", "inbox", "key-round", "layout-grid",
	"monitor", "network", "package", "phone", "printer", "server", "shield", "triangle-alert",
	"users", "wrench",
}

// IsPackageIcon says whether name is one of PackageIcons.
func IsPackageIcon(name string) bool { return slices.Contains(PackageIcons, name) }
