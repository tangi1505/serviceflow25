// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// UI46: the table is as high as its rows. Beside the open preview it stretched to the height of
// the preview; a box of its own height shows no vertical scroll bar for rows that fit. The test
// shows four records of its own through a saved search and opens the preview, in both row
// heights and at the scales of a Windows screen (125 % and 150 %): there the cells of the
// compact row reached half a pixel past the last row, which became a vertical bar (acceptance
// 2026-11-18).
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

test.afterAll(async ({ browser }) => {
  // The person's row height back to normal for the other tests.
  const page = await browser.newPage();
  await signIn(page);
  await page.request.patch("/api/v1/me/settings", { headers, data: { density: null } });
  await page.close();
});

for (const scale of [1, 1.25, 1.5]) {
  test.describe(`at a scale of ${scale}`, () => {
    test.use({ deviceScaleFactor: scale });

    for (const density of ["compact", "normal"] as const) {
      test(`the ${density} table keeps the height of its rows beside the preview`, async ({
        page,
      }) => {
        await signIn(page);
        const set = await page.request.patch("/api/v1/me/settings", { headers, data: { density } });
        expect(set.ok()).toBe(true);
        const stamp = `UI46 Höhe ${density} ${scale} ${Date.now()}`;
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
        await expect(rows.first()).toHaveCSS("height", density === "compact" ? "36px" : "52px");
        const box = page.locator("main div.overflow-auto").filter({ has: page.locator("tbody") });
        // The box ends below the fourth row: as high as the head, the rows and a horizontal bar;
        // nothing inside reaches past it.
        const fits = () =>
          box.evaluate((el: HTMLElement) => {
            const head = el.querySelector("thead")!.getBoundingClientRect().height;
            const body = el.querySelector("tbody")!.getBoundingClientRect().height;
            const bar = el.offsetHeight - el.clientHeight; // horizontal bar and border
            return {
              scrollHeight: el.scrollHeight,
              clientHeight: el.clientHeight,
              height: Math.round(el.getBoundingClientRect().height),
              rows: Math.round(head + body + bar),
            };
          });
        const alone = await fits();
        expect(alone.scrollHeight).toBeLessThanOrEqual(alone.clientHeight);
        expect(alone.height).toBe(alone.rows);

        await page.getByRole("row", { name: new RegExp(`${stamp} 1\\b`) }).click();
        const preview = page.getByRole("complementary", { name: "Vorschau" });
        await expect(preview.getByRole("heading", { name: `${stamp} 1` })).toBeVisible();
        const beside = await fits();
        expect(beside.scrollHeight).toBeLessThanOrEqual(beside.clientHeight);
        expect(beside.height).toBe(alone.height);
        // The preview is higher than the table, which does not grow with it.
        expect((await preview.boundingBox())!.height).toBeGreaterThan(beside.height);
      });
    }
  });
}
