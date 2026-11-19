// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI25 in a real browser: the administrator adds a field to Probe in the
// editor, by keyboard; it stands in the list and in the form of a new record without a
// reload; the export of the customer layer holds exactly this change; taking it back removes
// it again (the column and its values stay in the database). axe without findings.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

test("a field added in the editor is in list, form and export at once", async ({ page }) => {
  const stamp = Date.now();
  const label = `UI25 ${stamp}`;
  const field = `k_ui25_${stamp}`;
  await signIn(page);
  // Fields an earlier, broken run left behind are taken back first, newest first.
  const headers = { "X-Requested-With": "itsm" };
  const before = (await (await page.request.get("/api/v1/types/probe.probe/customer")).json()) as {
    version: number;
    changes: { id: string; field: string }[];
  };
  let version = before.version;
  for (const c of [...before.changes].reverse().filter((x) => x.field.startsWith("k_ui25_"))) {
    const res = await page.request.delete(
      `/api/v1/types/probe.probe/customer/${c.id}?version=${version}`,
      { headers },
    );
    expect(res.status()).toBe(200);
    version = ((await res.json()) as { version: number }).version;
  }
  await page.goto("/types/probe.probe");
  await page.locator("main").getByRole("button", { name: "Anpassen" }).click();
  await page.getByRole("menuitem", { name: /Typ anpassen/ }).click();
  await expect(page.getByRole("heading", { name: "Typ anpassen: Probe" })).toBeVisible();

  const form = page.getByRole("region", { name: "Neues Feld" });
  await form.getByLabel("Beschriftung", { exact: true }).focus();
  await page.keyboard.type(label);
  await expect(form.getByLabel("Technischer Name")).toHaveValue(field);
  await form.getByRole("button", { name: "Feld an Probe anlegen" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByText(`Feld „${label}“ angelegt.`)).toBeVisible();
  const fields = page.getByRole("region", { name: "Felder" });
  await expect(fields.getByRole("listitem", { name: label })).toBeVisible();
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(results.violations.map((v) => v.id)).toEqual([]);

  // The export holds the change: the field with its name, made by this account.
  const exported = (await (await page.request.get("/api/v1/customer")).json()) as {
    type: string;
    changes: { op: string; field: string; by_name?: string }[];
  }[];
  const probe = exported.find((l) => l.type === "probe.probe");
  expect(probe?.changes.filter((c) => c.field === field)).toEqual([
    expect.objectContaining({ op: "add_field", field, by_name: "Lasttest" }),
  ]);

  // In the list and in the form, without a reload.
  await page.getByRole("link", { name: "Zur Liste" }).click();
  await expect(page.getByRole("columnheader", { name: new RegExp(label) })).toBeVisible();
  await page.getByRole("button", { name: "Neu", exact: true }).click();
  await expect(page.getByLabel(new RegExp(`^${label}`))).toBeVisible();

  // Taking it back leaves the layer as it was.
  await page.goto("/types/probe.probe/customize");
  const changes = page.getByRole("region", { name: "Änderungen" });
  await changes
    .getByRole("button", { name: `Zurücknehmen: Feld „${label}“ (${field}) angelegt` })
    .click();
  // The common question first (UI64).
  await page.getByRole("alertdialog").getByRole("button", { name: "Zurücknehmen" }).click();
  await expect(
    page.getByText(`Zurückgenommen: Feld „${label}“ (${field}) angelegt.`),
  ).toBeVisible();
  await expect(fields.getByRole("listitem", { name: label })).toHaveCount(0);
});
