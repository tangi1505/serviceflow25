// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI39 in a real browser (UI plan E51): a new name shows in the header, in the
// title of the browser and on the sign-in page; a colour below 4.5 : 1 on white is an error at
// the field and nothing is saved; with "Akzentfarbe" switched off another person no longer finds
// the choice in Meine Einstellungen; axe without findings. The account markenpfleger
// (scripts/ci.sh) gives the root its usual appearance back at the end.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };
const usual = {
  "ui.app_name": null,
  "ui.accent": null,
  "ui.primary_accent": null,
  "ui.own_accent": null,
};

async function signIn(page: Page, user: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill(user);
  await page.getByLabel("Kennwort").fill(password);
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

async function axe(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`),
  ).toEqual([]);
}

test.afterAll(async ({ browser }) => {
  const page = await (await browser.newContext()).newPage();
  await signIn(page, "markenpfleger", "Eigene-Marke39!");
  await page.request.patch("/api/v1/org/nodes/root/settings", { headers, data: usual });
});

test("name, colour and what persons may choose, for the whole organisation", async ({
  page,
  browser,
}) => {
  await signIn(page, "markenpfleger", "Eigene-Marke39!");
  await page.request.patch("/api/v1/org/nodes/root/settings", { headers, data: usual });
  await page.goto("/appearance");
  await expect(page.getByRole("heading", { level: 1, name: "Erscheinungsbild" })).toBeVisible();

  await page.getByLabel("Name der Anwendung").fill("Leitstelle UI39");
  // 4.48 : 1 is too weak: an error at the field, and Speichern does nothing.
  const color = page.getByRole("textbox", { name: "Akzentfarbe", exact: true });
  await color.fill("#777777");
  await expect(page.getByText(/zu schwach, mindestens 4,5 : 1/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Speichern" })).toBeDisabled();
  await color.fill("#1F5FA8");
  await expect(page.getByText(/erfüllt AA/)).toBeVisible();
  await page.getByRole("checkbox", { name: "Akzentfarbe", exact: true }).uncheck();
  await axe(page);
  await page.getByRole("button", { name: "Speichern" }).click();
  await expect(page.getByText("Gespeichert. Gilt ab jetzt für alle.")).toBeVisible();
  await expect(page.getByRole("banner").getByText("Leitstelle UI39")).toBeVisible();
  await expect(page).toHaveTitle("Leitstelle UI39");

  // Another person: the sign-in page already shows the name, the choice of the accent is gone.
  const other = await (await browser.newContext()).newPage();
  await other.goto("/login");
  await expect(other.getByText("Leitstelle UI39")).toBeVisible();
  await signIn(other, "kollegin", "Zweite-Sicht8!");
  await other.goto("/settings");
  await other.getByRole("tab", { name: "Darstellung" }).click();
  await expect(other.getByRole("button", { name: "Dunkel" })).toBeVisible();
  await expect(other.getByRole("button", { name: "Indigo" })).toHaveCount(0);
  await expect(other.getByText("Stellt deine Organisation ein.")).toBeVisible();
});
