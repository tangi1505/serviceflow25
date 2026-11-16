// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI1 in a real browser: sign-in page, navigation after sign-in, a theme with
// another primary colour reaches the components, axe without findings on both pages (E42).
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const user = process.env["ITSM_E2E_USER"] ?? "load";
const password = process.env["ITSM_E2E_PASSWORD"] ?? "Messlauf-Pw7!";

async function expectNoViolations(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map(
      (v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(" ")).join(", ")})`,
    ),
  ).toEqual([]);
}

test("sign in, navigate, sign out", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("heading", { name: "Anmelden" })).toBeVisible();
  await expectNoViolations(page);

  // The theme is no choice of a person any more (UI38, E49): the sign-in page has none.
  const button = page.getByRole("button", { name: "Anmelden" });
  await expect(page.getByLabel("Theme")).toHaveCount(0);

  await page.getByLabel("Benutzername").fill(user);
  await page.getByLabel("Kennwort").fill(password);
  await button.click();

  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("navigation", { name: "Typen" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Probe" })).toBeVisible();
  await expectNoViolations(page);

  await page.getByRole("link", { name: "Probe" }).click();
  await expect(page).toHaveURL(/\/types\/probe\.probe$/);
  await expect(page.getByRole("heading", { name: "Probe", exact: true })).toBeVisible();
  await expectNoViolations(page);

  // Signing out sits in the account menu of the header (UI35).
  await page.getByRole("button", { name: /^Konto von / }).click();
  await page.getByRole("button", { name: "Abmelden" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
});
