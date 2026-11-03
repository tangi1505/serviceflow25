// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI27 in a real browser with two people: the administrator narrows the role
// of the second one in the matrix of the role editor from "alle" to "eigene"; the second one
// sees fewer records in the list of Probe at once, after reloading; back to "alle", the
// records are there again. axe without findings on the editor. The CI gives the account
// pruefer the role e2e_rollen (probe read all, workplace).
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };

async function signIn(page: Page, user: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill(user);
  await page.getByLabel("Kennwort").fill(password);
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

// count reads the number of records the list says it loaded.
async function count(page: Page): Promise<number> {
  await page.goto("/types/probe.probe");
  const status = page.getByText(/^\d+\+? Datensätze$/);
  await expect(status).toBeVisible();
  return Number(/^\d+/.exec((await status.textContent()) ?? "")![0]);
}

test("narrowing a role in the matrix shows the person fewer records at once", async ({
  browser,
}) => {
  const admin = await (await browser.newContext()).newPage();
  const holder = await (await browser.newContext()).newPage();
  await signIn(admin, "load", "Messlauf-Pw7!");
  await signIn(holder, "pruefer", "Rollen-Pruef11!");

  // A record of the administrator: not the holder's own.
  const created = await admin.request.post("/api/v1/records/probe.probe", {
    headers,
    data: { title: `UI27 ${Date.now()}`, priority: 2 },
  });
  expect(created.status()).toBe(201);
  const before = await count(holder);
  expect(before).toBeGreaterThan(0);

  // The role editor from the navigation.
  await admin.goto("/");
  await admin.getByRole("link", { name: "Rollen und Rechte" }).click();
  await admin.getByRole("link", { name: "Rollenprüfung (Test)" }).click();
  await expect(admin.getByRole("heading", { name: "Rolle: Rollenprüfung (Test)" })).toBeVisible();
  const read = admin.getByRole("combobox", { name: "Probe: lesen" });
  await expect(read).toHaveValue("all");
  const results = await new AxeBuilder({ page: admin }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`),
  ).toEqual([]);

  await read.selectOption("own");
  await expect(admin.getByText("Probe: lesen: eigene.")).toBeVisible();
  expect(await count(holder)).toBe(0);

  // Back to all: the records are there again.
  await admin.getByRole("combobox", { name: "Probe: lesen" }).selectOption("all");
  await expect(admin.getByText("Probe: lesen: alle.")).toBeVisible();
  expect(await count(holder)).toBe(before);
});
