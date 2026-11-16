// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI37 in a real browser (UI plan E50): the star at "Probe" pins it under
// Favoriten; in Meine Einstellungen › Darstellung "Stammdaten" goes before "Dashboards" and the
// area "Probe" is hidden; a second browser of the same account shows it so too, and the hidden
// list stays reachable with Strg K; axe without findings. The account anordner (scripts/ci.sh)
// gets the usual arrangement back at the end.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };
const usual = { nav: null, nav_order: null, nav_hidden: null, favorites: null };

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("anordner");
  await page.getByLabel("Kennwort").fill("Eigene-Ordnung37!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

// areas are the names of the open areas of the bar, top down.
async function areas(page: Page): Promise<string[]> {
  return page
    .getByRole("navigation", { name: "Typen" })
    .locator("button[aria-expanded]")
    .allTextContents();
}

async function axe(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`),
  ).toEqual([]);
}

test.afterAll(async ({ browser }) => {
  const page = await (await browser.newContext()).newPage();
  await signIn(page);
  await page.request.patch("/api/v1/me/settings", { headers, data: usual });
});

test("favourites and the own order hold in a second browser", async ({ page, browser }) => {
  await signIn(page);
  await page.request.patch("/api/v1/me/settings", { headers, data: usual });
  await page.reload();
  const nav = page.getByRole("navigation", { name: "Typen" });
  await expect(nav.getByRole("link", { name: "Probe", exact: true })).toBeVisible();

  // The star shows on pointing at the entry and pins it on top.
  await nav.getByRole("link", { name: "Probe", exact: true }).hover();
  await nav.getByRole("button", { name: "Unter Favoriten anheften: Probe", exact: true }).click();
  await expect(nav.getByRole("button", { name: "Favoriten", exact: true })).toBeVisible();
  await expect(nav.getByRole("link", { name: "Probe", exact: true })).toHaveCount(2);
  expect((await areas(page))[0]).toBe("Favoriten");
  await axe(page);

  await page.goto("/settings");
  await page.getByRole("tab", { name: "Darstellung" }).click();
  const up = page.getByRole("button", { name: "Stammdaten nach oben", exact: true });
  while ((await areas(page)).indexOf("Stammdaten") > (await areas(page)).indexOf("Dashboards")) {
    const before = await areas(page);
    await up.click();
    await expect.poll(() => areas(page)).not.toEqual(before);
  }
  await page.getByRole("checkbox", { name: "Probe anzeigen", exact: true }).uncheck();
  await expect.poll(() => areas(page)).not.toContain("Probe");
  await axe(page);

  // A second browser of the same account: the same bar.
  const other = await (await browser.newContext()).newPage();
  await signIn(other);
  await expect(
    other
      .getByRole("navigation", { name: "Typen" })
      .getByRole("button", { name: "Favoriten", exact: true }),
  ).toBeVisible();
  const shown = await areas(other);
  expect(shown).not.toContain("Probe");
  expect(shown.indexOf("Stammdaten")).toBeLessThan(shown.indexOf("Dashboards"));
  // The favourite of the hidden area stays, and Strg K still leads to the list.
  await expect(
    other
      .getByRole("navigation", { name: "Typen" })
      .getByRole("link", { name: "Probe", exact: true }),
  ).toHaveCount(1);
  await other.keyboard.press("Control+K");
  const dialog = other.getByRole("dialog", { name: "Befehlszeile" });
  await other.keyboard.type("Probe");
  await dialog
    .getByRole("group", { name: "Gehe zu" })
    .getByRole("option", { name: /^Probe/ })
    .first()
    .click();
  await expect(other).toHaveURL(/\/types\/probe\.probe$/);
});
