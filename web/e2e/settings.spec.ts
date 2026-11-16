// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI9 in a real browser: language English in "Meine Einstellungen", every text
// English, also after signing out and in again; axe without findings. The test has an account
// of its own and gives it its German back at the end.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const who = { user: "einsteller", password: "Eigene-Wahl9!" };

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel(/Benutzername|Username/).fill(who.user);
  await page.getByLabel(/Kennwort|Password/).fill(who.password);
  await page.getByRole("button", { name: /^(Anmelden|Sign in)$/ }).click();
  await expect(page).toHaveURL(/\/$/);
}

test("English in my settings holds after signing in again", async ({ page }) => {
  await signIn(page);
  // Start from German, whatever an earlier run left.
  await page.request.patch("/api/v1/me/settings", {
    headers: { "X-Requested-With": "itsm" },
    data: { language: "de" },
  });
  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "Meine Einstellungen" })).toBeVisible();
  await page.locator("main").getByLabel("Sprache").selectOption("en");
  await page.locator("main").getByRole("button", { name: "Speichern" }).click();
  await expect(page.getByRole("heading", { name: "My settings" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Types" })).toBeVisible();
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(results.violations.map((v) => v.id)).toEqual([]);

  // Signing out sits in the account menu of the header (UI35).
  await page.getByRole("button", { name: /^Account of / }).click();
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page).toHaveURL(/\/login$/);
  // The browser speaks German: the sign-in page is German, the person's setting comes after.
  await page.reload();
  await expect(page.getByLabel("Benutzername")).toBeVisible();
  await signIn(page);
  await expect(page.getByRole("navigation", { name: "Types" })).toBeVisible();
  await expect(page.getByRole("button", { name: /^Account of / })).toBeVisible();

  await page.request.patch("/api/v1/me/settings", {
    headers: { "X-Requested-With": "itsm" },
    data: { language: "de" },
  });
});
