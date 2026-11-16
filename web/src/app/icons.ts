// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The symbols a package may name for its area (UI36, UI plan E48), a reading area of the
// portal (UI41) and a tile (UI42, UI plan E53): the same fixed list as meta.PackageIcons in the
// core, in the same order (TestIconsOfWeb holds them equal). A package without one gets the box.
import {
  BookOpen,
  Box,
  Calendar,
  ChartColumn,
  CircleHelp,
  ClipboardList,
  Database,
  FileText,
  FlaskConical,
  Folder,
  Headset,
  Inbox,
  KeyRound,
  LayoutGrid,
  Monitor,
  Network,
  Package,
  Phone,
  Printer,
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
  "circle-help": CircleHelp,
  "clipboard-list": ClipboardList,
  database: Database,
  "file-text": FileText,
  "flask-conical": FlaskConical,
  folder: Folder,
  headset: Headset,
  inbox: Inbox,
  "key-round": KeyRound,
  "layout-grid": LayoutGrid,
  monitor: Monitor,
  network: Network,
  package: Package,
  phone: Phone,
  printer: Printer,
  server: Server,
  shield: Shield,
  "triangle-alert": TriangleAlert,
  users: Users,
  wrench: Wrench,
};

export function packageIcon(name: string | undefined): LucideIcon {
  return (name && packageIcons[name]) || Box;
}
