// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI10 in a real browser: a person without the right workplace lands in the
// portal and does not reach the workplace, not by address either; she creates a record with
// the fields of the portal, answers in its timeline and finds it under her records; axe
// without findings. The CI gives the account nutzerin the role e2e_portal. Since UI41 also the
// head of the portal: over the full width, the account menu, the areas to swipe on a phone.
import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("a user stays in the portal and creates there", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("nutzerin");
  await page.getByLabel("Kennwort").fill("Nur-Portal10!");
  await page.getByRole("button", { name: "Anmelden" }).click();
  await expect(page).toHaveURL(/\/portal$/);
  await expect(page.getByRole("heading", { name: "Meine Vorgänge" })).toBeVisible();

  // The workplace by address: back to the portal.
  await page.goto("/types/probe.probe");
  await expect(page).toHaveURL(/\/portal$/);
  // The account menu of the portal (UI41) has no way to the workplace without the right.
  await page.getByRole("button", { name: /^Konto von / }).click();
  await expect(page.getByRole("link", { name: "Meine Einstellungen" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Zum Arbeitsplatz" })).toHaveCount(0);
  await page.keyboard.press("Escape");

  // "Meine Vorgänge" is one list since UI43, without a card and a "Neu" per type.
  const start = page.getByRole("region", { name: "Meine Vorgänge" });
  await expect(start).toBeVisible();
  await expect(page.getByRole("region", { name: "Probe" })).toHaveCount(0);
  await expect(start.getByRole("link", { name: "Neu" })).toHaveCount(0);
  const before = Number((await start.getByText(/^\d+ offen$/).textContent())!.split(" ")[0]);

  const title = `UI10 ${Date.now()}`;
  await page.goto("/portal/probe.probe/new");
  await expect(page.getByLabel("Priorität")).toHaveCount(0);
  await page.getByLabel(/Titel/).fill(title);
  await page.getByRole("button", { name: "Anlegen" }).click();
  await expect(page.getByRole("heading", { name: title })).toBeVisible();
  await expect(page).toHaveURL(/\/portal\/probe\.probe\//);

  const timeline = page.getByRole("region", { name: "Zeitleiste" });
  await expect(timeline.getByLabel("Privat (nur für Berechtigte)")).toHaveCount(0);
  await timeline
    .getByRole("textbox", { name: "Neuer Eintrag", exact: true })
    .fill("Bitte bis Freitag.");
  await timeline.getByRole("button", { name: "Eintragen" }).click();
  await expect(timeline).toContainText("Bitte bis Freitag.");
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(results.violations.map((v) => v.id)).toEqual([]);

  // The head always has the areas since WF6 ("Genehmigungen" for everyone); the breadcrumb leads
  // back as before.
  await page
    .getByRole("navigation", { name: "breadcrumb" })
    .getByRole("link", { name: "Meine Vorgänge" })
    .click();
  // The new record stands first, with its number and type, and counts as open (UI43).
  const mine = page.getByRole("region", { name: "Meine Vorgänge" });
  await expect(mine.getByText(`${before + 1} offen`)).toBeVisible();
  const first = mine.getByRole("listitem").first();
  await expect(first.getByRole("link", { name: title })).toBeVisible();
  await expect(first).toContainText("Probe");
  // The running number with its prefix, as in the lists of the workplace.
  await expect(first).toContainText(/PRB-\d+/);
  await expect(first).toContainText("Neu");
  await mine.getByRole("button", { name: "Alle" }).click();
  await expect(mine.getByRole("button", { name: "Alle" })).toHaveAttribute("aria-pressed", "true");
  await expect(mine.getByRole("link", { name: title })).toBeVisible();
  const list = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(list.violations.map((v) => v.id)).toEqual([]);

  // The head of the portal (UI41, UI plan E43): from edge to edge, without a labelled select;
  // at 375 px the areas are a row to swipe and the head does not wrap the account away.
  await page.setViewportSize({ width: 1440, height: 900 });
  const head = page.getByRole("banner");
  expect((await head.boundingBox())!.width).toBe(1440);
  await expect(head.getByRole("combobox")).toHaveCount(0);
  await page.setViewportSize({ width: 375, height: 800 });
  const areas = page.getByRole("navigation", { name: "Bereiche" });
  const row = areas.locator("xpath=..");
  expect(await row.evaluate((el) => getComputedStyle(el).overflowX)).toBe("auto");
  const brand = (await head.getByRole("link").first().boundingBox())!;
  const account = (await head.getByRole("button", { name: /^Konto von / }).boundingBox())!;
  expect(Math.abs(account.y - brand.y)).toBeLessThan(16);
  expect((await areas.boundingBox())!.y).toBeGreaterThan(brand.y + brand.height - 1);
  const phone = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(phone.violations.map((v) => v.id)).toEqual([]);
});
