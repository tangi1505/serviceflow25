// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI66 in a real browser: Verwaltung › Benutzer, a new user with a one-time
// password, who must change it at the first sign-in; a role added at the user shows at the
// role; blocked after the question, the sign-in is refused; axe without findings. The CI has no
// mail server, so the invitation is proven by the Go test of PL77 and the Vitest of the page.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { pick } from "./pick";

const user = process.env["ITSM_E2E_USER"] ?? "load";
const password = process.env["ITSM_E2E_PASSWORD"] ?? "Messlauf-Pw7!";

async function signIn(page: Page, name: string, pw: string) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill(name);
  await page.getByLabel("Kennwort").fill(pw);
  await page.getByRole("button", { name: "Anmelden" }).click();
}

async function noFindings(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(results.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
}

test("a user with a one-time password, a role and blocked", async ({ page, browser }) => {
  await signIn(page, user, password);
  await expect(page).toHaveURL(/\/$/);
  const stamp = String(Date.now()).slice(-8);
  const username = `ui66_${stamp}`;
  const lastName = `Prüfung ${stamp}`;

  await page.getByRole("link", { name: "Benutzer", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Benutzer", level: 1 })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "load" }).first()).toBeVisible();
  await noFindings(page);

  await page.getByRole("button", { name: "Neuer Benutzer" }).click();
  await page.getByLabel("Vorname").fill("Uta");
  await page.getByLabel(/^Nachname/).fill(lastName);
  await page.getByLabel(/^Benutzername/).fill(username);
  await page.getByRole("radio", { name: /Einmal-Kennwort/ }).check();
  await page.getByRole("textbox", { name: /^Einmal-Kennwort/ }).fill("Einmal-Pw7!x");
  await noFindings(page);
  await page.getByRole("button", { name: "Anlegen", exact: true }).click();
  await expect(page.getByRole("heading", { name: `Uta ${lastName}`, level: 1 })).toBeVisible();

  // A role at the user shows at the role.
  const roles = page.getByRole("region", { name: "Rollen" });
  await pick(roles.getByRole("combobox", { name: "Rolle" }), "e2e_rollen");
  await roles.getByRole("button", { name: "Rolle hinzufügen" }).click();
  await expect(roles.getByRole("table")).toContainText("Rollenprüfung (Test)");
  await noFindings(page);
  const back = page.url();
  await page.goto("/roles/e2e_rollen");
  await page.getByRole("tab", { name: /^Zuweisungen/ }).click();
  await expect(page.getByText(lastName, { exact: false }).first()).toBeVisible();

  // The first sign-in asks for a new password.
  const other = await browser.newContext();
  const second = await other.newPage();
  await signIn(second, username, "Einmal-Pw7!x");
  await expect(second).toHaveURL(/\/password/);
  await other.close();

  // Blocked after the question: the sign-in is refused.
  await page.goto(back);
  await page.getByRole("button", { name: "Aktionen" }).click();
  await page.getByRole("menuitem", { name: "Sperren" }).click();
  await expect(page.getByRole("alertdialog", { name: `„Uta ${lastName}“ sperren?` })).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.getByText(/gesperrt/).first()).toBeVisible();
  const third = await browser.newContext();
  const blocked = await third.newPage();
  await signIn(blocked, username, "Einmal-Pw7!x");
  await expect(blocked.getByRole("alert")).toBeVisible();
  await expect(blocked).toHaveURL(/\/login/);
  await third.close();
});
