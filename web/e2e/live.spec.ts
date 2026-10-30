// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI8 in two browsers with two people: B sees that A is editing the record,
// and A's saved change appears in B without reloading; axe without findings.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const people = {
  a: {
    user: process.env["ITSM_E2E_USER"] ?? "load",
    password: process.env["ITSM_E2E_PASSWORD"] ?? "Messlauf-Pw7!",
  },
  b: { user: "kollegin", password: "Zweite-Sicht8!" },
};

async function signIn(page: Page, who: { user: string; password: string }) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill(who.user);
  await page.getByLabel("Kennwort").fill(who.password);
  await page.getByRole("button", { name: "Anmelden" }).click();
  await expect(page).toHaveURL(/\/$/);
}

test("a change in one window appears in the other, with who is editing", async ({ browser }) => {
  test.setTimeout(90_000);
  const a = await (await browser.newContext()).newPage();
  const b = await (await browser.newContext()).newPage();
  await signIn(a, people.a);
  await signIn(b, people.b);
  const title = `UI8 ${Date.now()}`;
  const created = await a.request.post("/api/v1/records/probe.probe", {
    headers: { "X-Requested-With": "itsm" },
    data: { title },
  });
  expect(created.status()).toBe(201);
  const id = ((await created.json()) as { id: string }).id;
  await a.goto(`/types/probe.probe/${id}`);
  await b.goto(`/types/probe.probe/${id}`);
  await expect(b.getByRole("heading", { name: title })).toBeVisible();

  // A changes the title without saving: B learns it with its next beat (every 20 s).
  await a.getByLabel(/Titel/).fill(`${title} (A)`);
  await expect(b.getByText("Lasttest bearbeitet gerade")).toBeVisible({ timeout: 30_000 });
  const results = await new AxeBuilder({ page: b }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(results.violations.map((v) => v.id)).toEqual([]);

  // A saves: B shows the new title without reloading.
  await a.getByRole("button", { name: "Speichern" }).click();
  await expect(a.getByText("Gespeichert.")).toBeVisible();
  await expect(b.getByRole("heading", { name: `${title} (A)` })).toBeVisible({ timeout: 10_000 });
  await expect(b.getByRole("region", { name: "Zeitleiste" })).toContainText(
    `${title} → ${title} (A)`,
  );
});
