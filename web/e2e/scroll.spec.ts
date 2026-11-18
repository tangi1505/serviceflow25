// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The big list (UI2 Prüfweg): with scripts/seed-probe.sql loaded (100,000 records) the list
// scrolls page by page, keeps only a window of rows in the DOM and loads the next page by key.
// Runs only when ITSM_E2E_SCROLL is set, because it needs the seeded data:
//
//   docker compose exec -T postgres psql -U itsm -d itsm -v n=100000 < scripts/seed-probe.sql
//   ITSM_E2E_SCROLL=1 ITSM_E2E_URL=http://localhost:8092 pnpm e2e --grep scroll
import { expect, test } from "@playwright/test";

const user = process.env["ITSM_E2E_USER"] ?? "load";
const password = process.env["ITSM_E2E_PASSWORD"] ?? "Messlauf-Pw7!";

test("scrolls through a list of 100,000 records", async ({ page }) => {
  test.skip(!process.env["ITSM_E2E_SCROLL"], "needs the seeded data (ITSM_E2E_SCROLL=1)");
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill(user);
  await page.getByLabel("Kennwort").fill(password);
  await page.getByRole("button", { name: "Anmelden" }).click();
  await expect(page).toHaveURL(/\/$/);
  await page.goto("/types/probe.probe");
  await expect(page.getByText(/^\d+\+ Datensätze$/)).toBeVisible();

  const scroller = page.locator("div.overflow-auto").first();
  const timings: number[] = [];
  let loaded = 0;
  for (let i = 0; i < 40; i++) {
    const started = Date.now();
    await scroller.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
    });
    // The count grows when the next page arrived.
    await expect
      .poll(async () => {
        const text = await page.getByText(/^\d+\+? Datensätze$/).textContent();
        return Number((text ?? "0").replace(/\D/g, ""));
      })
      .toBeGreaterThan(loaded);
    const text = await page.getByText(/^\d+\+? Datensätze$/).textContent();
    loaded = Number((text ?? "0").replace(/\D/g, ""));
    timings.push(Date.now() - started);
    const rows = await page.locator("tbody tr").count();
    expect(rows).toBeLessThan(80);
  }
  timings.sort((a, b) => a - b);
  const p95 = timings[Math.floor(timings.length * 0.95)];
  console.log(
    `Scroll: ${loaded} Datensätze geladen, Nachladen je Seite p50 ${timings[Math.floor(timings.length / 2)]} ms, p95 ${p95} ms`,
  );
  expect(loaded).toBeGreaterThanOrEqual(4000);
  expect(p95).toBeLessThan(500);
});
