// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI30 in a real browser: "Kompakt" makes the rows 36 px and holds after a
// reload; a click on a row shows the preview, "Öffnen" leads to the record page; at 375 px the
// navigation is a drawer (UI40, before: above the list); axe without findings with the preview
// and with the drawer open. The account
// gestalter (scripts/ci.sh) goes back to the normal row height at the end.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("gestalter");
  await page.getByLabel("Kennwort").fill("Dunkel-Hell29!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

test.afterAll(async ({ browser }) => {
  const page = await (await browser.newContext()).newPage();
  await signIn(page);
  await page.request.patch("/api/v1/me/settings", { headers, data: { density: null } });
});

test("compact rows, the preview beside the list, the navigation on a phone", async ({ page }) => {
  await signIn(page);
  const title = `UI30 Vorschau ${Date.now()}`;
  const created = await page.request.post("/api/v1/records/probe.probe", {
    headers,
    data: { title, priority: 2 },
  });
  const id = ((await created.json()) as { id: string }).id;

  // The newest record is the first row (sorted by the last change).
  await page.goto("/types/probe.probe");
  const row = page.getByRole("row", { name: new RegExp(title) });
  await expect(row).toBeVisible();
  const height = async () => (await row.boundingBox())!.height;
  expect(await height()).toBe(52);
  // The row height is in the menu "Ansicht" of the bar (UI48).
  await page.getByRole("button", { name: "Ansicht" }).click();
  await page
    .getByRole("group", { name: "Zeilenhöhe" })
    .getByRole("menuitemradio", { name: "Kompakt" })
    .click();
  await expect(page.getByRole("menuitemradio", { name: "Kompakt", checked: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect.poll(height).toBe(36);
  await page.reload();
  await expect(page.getByRole("row", { name: new RegExp(title) })).toBeVisible();
  expect(await height()).toBe(36);

  // The preview beside the list; axe with it open.
  await page.getByRole("row", { name: new RegExp(title) }).click();
  const preview = page.getByRole("complementary", { name: "Vorschau" });
  await expect(preview.getByRole("heading", { name: title })).toBeVisible();
  await expect(preview.getByText("P2")).toBeVisible();
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`),
  ).toEqual([]);
  await preview.getByRole("link", { name: "Öffnen" }).click();
  await expect(page).toHaveURL(new RegExp(`/types/probe.probe/${id}$`));
  await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();

  // On a phone the navigation is a drawer over the list (UI40, UI plan E42; before UI40 it stood
  // above the list): opened by the button of the header, closed with Esc, the focus back.
  await page.setViewportSize({ width: 375, height: 800 });
  await page.goto("/types/probe.probe");
  await expect(page.getByRole("row", { name: new RegExp(title) })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Typen" })).toHaveCount(0);
  const opener = page.getByRole("button", { name: "Navigation öffnen" });
  await opener.click();
  const drawer = page.getByRole("navigation", { name: "Typen" });
  await expect(drawer).toBeVisible();
  await expect(drawer.getByRole("button", { name: "Navigation schließen" })).toBeFocused();
  const box = (await drawer.boundingBox())!;
  expect(box.x).toBe(0);
  expect(box.y).toBe(0);
  expect(box.width).toBeGreaterThan(300);
  const drawerAxe = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    drawerAxe.violations.map(
      (v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`,
    ),
  ).toEqual([]);
  await page.keyboard.press("Escape");
  await expect(drawer).toHaveCount(0);
  await expect(opener).toBeFocused();
});
