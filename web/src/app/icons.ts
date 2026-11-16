// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The symbols a package may name for its area (UI36, UI plan E48): the same fixed list as
// meta.PackageIcons in the core. A package without one gets the box.
import {
  BookOpen,
  Box,
  Calendar,
  ChartColumn,
  ClipboardList,
  Database,
  FileText,
  FlaskConical,
  Folder,
  Headset,
  Inbox,
  LayoutGrid,
  Network,
  Package,
  Server,
  Shield,
  TriangleAlert,
  Users,
  Wrench,
  type LucideIcon,
} from "lucide-react";

export const packageIcons: Record<string, LucideIcon> = {
  "book-open": BookOpen,
  box: Box,
  calendar: Calendar,
  "chart-column": ChartColumn,
  "clipboard-list": ClipboardList,
  database: Database,
  "file-text": FileText,
  "flask-conical": FlaskConical,
  folder: Folder,
  headset: Headset,
  inbox: Inbox,
  "layout-grid": LayoutGrid,
  network: Network,
  package: Package,
  server: Server,
  shield: Shield,
  "triangle-alert": TriangleAlert,
  users: Users,
  wrench: Wrench,
};

export function packageIcon(name: string | undefined): LucideIcon {
  return (name && packageIcons[name]) || Box;
}
