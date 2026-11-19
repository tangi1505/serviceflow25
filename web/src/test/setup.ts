// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Test setup for Vitest: DOM matchers, cleanup, and the accessibility check with axe-core
// (Hauptplan E42: one violation fails the build).
import "@testing-library/jest-dom/vitest";

import { cleanup } from "@testing-library/react";
import axe from "axe-core";
import { afterEach, expect } from "vitest";

import { clearConfirm } from "@/ui/confirm";
import { toast } from "@/ui/toast";

afterEach(() => {
  cleanup();
  // The messages live outside React; each test starts without (UI59).
  toast.clear();
  clearConfirm();
});

// jsdom has no layout; the router's scroll restoration calls scrollTo on every navigation.
Object.defineProperty(window, "scrollTo", { value: () => undefined, writable: true });

// expectAccessible runs axe on a rendered container and fails with every violation listed.
export async function expectAccessible(container: Element): Promise<void> {
  const results = await axe.run(container, {
    // Colour contrast needs real layout; jsdom has none. Playwright checks it in the browser.
    rules: { "color-contrast": { enabled: false } },
  });
  const report = results.violations.map(
    (v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(" ")).join(", ")})`,
  );
  expect(report).toEqual([]);
}
