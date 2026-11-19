// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// UI57 (UI-Plan E59): everything in the page header and the bar of a list is 32 px high, the
// filter row too: the saved searches ("Alle Datensätze"), the filter row, "Ansicht",
// "Exportieren", "Anpassen" and "Neu" take the height from the token --control-bar.
import { expect, test, type Page } from "@playwright/test";

test.use({ viewport: { width: 1400, height: 900 } });

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

test("the controls of the bar of a list are all 32 px high", async ({ page }) => {
  await signIn(page);
  await page.goto("/types/probe.probe");
  await expect(page.getByRole("grid")).toBeVisible();
  const controls = await page.locator("main").evaluate((main) =>
    [...main.querySelectorAll<HTMLElement>('[class*="--control-bar"]')]
      // The field inside the filter row is the row less its border.
      .filter((el) => el.offsetParent !== null && !el.className.includes("calc("))
      .map((el) => ({
        name: (el.getAttribute("aria-label") ?? el.textContent ?? "").trim().slice(0, 40),
        height: el.getBoundingClientRect().height,
      })),
  );
  // Saved searches, filter row, Ansicht, Exportieren, Anpassen, Neu.
  expect(controls.length).toBeGreaterThanOrEqual(6);
  for (const c of controls) {
    expect(c, c.name).toEqual({ name: c.name, height: 32 });
  }
});
