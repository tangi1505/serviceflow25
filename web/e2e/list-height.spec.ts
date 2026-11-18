// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// UI46: the table is as high as its rows. Beside the open preview it stretched to the height of
// the preview; a box of its own height shows no vertical scroll bar for rows that fit. The test
// shows four records of its own through a saved search and opens the preview.
import { expect, test, type Page } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };

test.use({ viewport: { width: 1700, height: 1250 } });

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

test("the table keeps the height of its rows beside the preview", async ({ page }) => {
  await signIn(page);
  const stamp = `UI46 Höhe ${Date.now()}`;
  for (let i = 1; i <= 4; i++) {
    const created = await page.request.post("/api/v1/records/probe.probe", {
      headers,
      data: { title: `${stamp} ${i}`, priority: 2 },
    });
    expect(created.ok()).toBe(true);
  }
  const search = await page.request.post("/api/v1/records/core.saved_search", {
    headers,
    data: {
      name: stamp,
      target_type: "probe.probe",
      query: JSON.stringify({ field: "title", contains: stamp }),
    },
  });
  expect(search.ok()).toBe(true);
  const searchID = ((await search.json()) as { id: string }).id;

  await page.goto(`/types/probe.probe?search=${searchID}`);
  const rows = page.getByRole("row", { name: new RegExp(stamp) });
  await expect(rows).toHaveCount(4);
  const box = page.locator("main div.overflow-auto").filter({ has: page.locator("tbody") });
  // The box ends below the fourth row: as high as the head, the rows and a horizontal bar.
  const fits = () =>
    box.evaluate((el: HTMLElement) => {
      const head = el.querySelector("thead")!.getBoundingClientRect().height;
      const body = el.querySelector("tbody")!.getBoundingClientRect().height;
      const bar = el.offsetHeight - el.clientHeight; // horizontal bar and border
      return {
        noVerticalBar: el.scrollHeight <= el.clientHeight,
        height: Math.round(el.getBoundingClientRect().height),
        rows: Math.round(head + body + bar),
      };
    });
  const alone = await fits();
  expect(alone.noVerticalBar).toBe(true);
  expect(alone.height).toBe(alone.rows);

  await page.getByRole("row", { name: new RegExp(`${stamp} 1\\b`) }).click();
  const preview = page.getByRole("complementary", { name: "Vorschau" });
  await expect(preview.getByRole("heading", { name: `${stamp} 1` })).toBeVisible();
  const beside = await fits();
  expect(beside.noVerticalBar).toBe(true);
  expect(beside.height).toBe(alone.height);
  // The preview is higher than the table, which does not grow with it.
  expect((await preview.boundingBox())!.height).toBeGreaterThan(beside.height);
});
