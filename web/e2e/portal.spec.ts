// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI10 in a real browser: a person without the right workplace lands in the
// portal and does not reach the workplace, not by address either; she creates a record with
// the fields of the portal, answers in its timeline and finds it under her records; axe
// without findings. The CI gives the account nutzerin the role e2e_portal.
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
  await expect(page.getByRole("link", { name: "Zum Arbeitsplatz" })).toHaveCount(0);

  const title = `UI10 ${Date.now()}`;
  const section = page.getByRole("region", { name: "Probe" });
  await section.getByRole("link", { name: "Neu" }).click();
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

  await page.getByRole("link", { name: "Meine Vorgänge" }).click();
  await expect(
    page.getByRole("region", { name: "Probe" }).getByRole("link", { name: title }),
  ).toBeVisible();
});
