// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// UI45: from 768 px only the content scrolls, never the whole document with the navigation.
// The dashboard "Statistik" scrolled past its last widget because the table behind a line chart
// (sr-only, absolutely placed) had no placed parent and stretched the document. The test builds
// a dashboard of its own with line charts below the fold (the CI has no package "sd") and checks
// dashboard, list and record page: the document is never higher than the window.
import { expect, test, type Page } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };

test.use({ viewport: { width: 1280, height: 720 } });

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

// documentFits is true when the document is not higher than the window.
async function documentFits(page: Page) {
  return page.evaluate(() => document.documentElement.scrollHeight <= window.innerHeight);
}

test("the document is never higher than the window (dashboard, list, record)", async ({ page }) => {
  await signIn(page);
  const stamp = Date.now();
  const record = await page.request.post("/api/v1/records/probe.probe", {
    headers,
    data: { title: `UI45 Höhe ${stamp}`, priority: 2 },
  });
  expect(record.ok()).toBe(true);
  const recordID = ((await record.json()) as { id: string }).id;

  const created = await page.request.post("/api/v1/records/core.dashboard", {
    headers,
    data: { name: `UI45 Höhe ${stamp}`, period: "all" },
  });
  expect(created.ok()).toBe(true);
  const board = (await created.json()) as { id: string; version: number };
  // Eight line charts in four rows: the lower ones and their tables lie below the fold.
  const widgets = Array.from({ length: 8 }, (_, i) => ({
    title: `Linie ${i + 1}`,
    kind: "line",
    target_type: "probe.probe",
    group_by: "created_at:day",
    x: (i % 2) * 2,
    y: Math.floor(i / 2) * 2,
    w: 2,
    h: 2,
  }));
  const saved = await page.request.put(`/api/v1/dashboards/${board.id}`, {
    headers,
    data: { version: board.version, widgets },
  });
  expect(saved.ok()).toBe(true);

  await page.goto(`/dashboards/${board.id}`);
  await expect(page.getByRole("img", { name: /Linie 8/ })).toBeAttached();
  // The content itself is higher than the window, so the test shows something.
  const main = page.locator("main");
  expect(await main.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
  expect(await documentFits(page)).toBe(true);
  // Scrolled to the end, the navigation stays where it is.
  const nav = page.getByRole("navigation").first();
  const navTop = (await nav.boundingBox())!.y;
  await main.evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  expect((await nav.boundingBox())!.y).toBe(navTop);

  await page.goto("/types/probe.probe");
  await expect(page.getByRole("row", { name: new RegExp(`UI45 Höhe ${stamp}`) })).toBeVisible();
  expect(await documentFits(page)).toBe(true);

  await page.goto(`/types/probe.probe/${recordID}`);
  await expect(page.getByRole("heading", { name: `UI45 Höhe ${stamp}` })).toBeVisible();
  expect(await documentFits(page)).toBe(true);

  await page.request.delete(`/api/v1/records/core.dashboard/${board.id}`, { headers });
});
