// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Browser tests against a running server (scripts/ci.sh starts one with the built interface);
// the address comes from ITSM_E2E_URL. Chromium only: the tool image carries just this browser.
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  retries: 0,
  reporter: [["list"]],
  use: {
    baseURL: process.env["ITSM_E2E_URL"] ?? "http://127.0.0.1:18082",
    locale: "de-DE",
    // No CSS transitions while axe measures colours (and a courtesy to people who ask for it).
    reducedMotion: "reduce",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
