// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI62 in a real browser: a record from its page (Aktionen › Löschen), a
// dashboard from its page and a saved search from its menu go to the trash without a question;
// the message at the bottom right offers "Rückgängig", which brings each back; axe without
// findings with the message open.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { pick } from "./pick";

const user = process.env["ITSM_E2E_USER"] ?? "load";
const password = process.env["ITSM_E2E_PASSWORD"] ?? "Messlauf-Pw7!";
const headers = { "X-Requested-With": "itsm" };

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill(user);
  await page.getByLabel("Kennwort").fill(password);
  await page.getByRole("button", { name: "Anmelden" }).click();
  await expect(page).toHaveURL(/\/$/);
}

async function create(page: Page, typeName: string, data: Record<string, unknown>) {
  const r = await page.request.post(`/api/v1/records/${typeName}`, { headers, data });
  expect(r.status()).toBe(201);
  return String(((await r.json()) as { id: string }).id);
}

// undo clicks "Rückgängig" in the message and waits until the record is back.
async function undo(page: Page, path: string) {
  const note = page
    .locator('[role="status"], [role="alert"]')
    .filter({ hasText: "In den Papierkorb gelegt" })
    .last();
  await expect(note).toBeVisible();
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(results.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
  await note.getByRole("button", { name: "Rückgängig" }).click();
  await expect(page.getByText("Aus dem Papierkorb zurückgeholt")).toBeVisible();
  expect((await page.request.get(path, { headers })).status()).toBe(200);
}

test("delete a record, a dashboard and a saved search, and take each back", async ({ page }) => {
  await signIn(page);
  const stamp = Date.now();
  const ids: [string, string][] = [];
  try {
    const rec = await create(page, "probe.probe", { title: `UI62 ${stamp}` });
    ids.push(["probe.probe", rec]);
    await page.goto(`/types/probe.probe/${rec}`);
    await page.getByRole("button", { name: "Aktionen" }).click();
    await page.getByRole("menuitem", { name: "Löschen" }).click();
    await expect(page).toHaveURL(/\/types\/probe\.probe$/);
    expect(
      (await page.request.get(`/api/v1/records/probe.probe/${rec}`, { headers })).status(),
    ).toBe(404);
    await undo(page, `/api/v1/records/probe.probe/${rec}`);

    const dash = await create(page, "core.dashboard", { name: `UI62 ${stamp}` });
    ids.push(["core.dashboard", dash]);
    await page.goto(`/dashboards/${dash}`);
    await page.getByRole("button", { name: "Löschen" }).click();
    await expect(page).toHaveURL(/\/$/);
    await undo(page, `/api/v1/records/core.dashboard/${dash}`);

    const search = await create(page, "core.saved_search", {
      name: `UI62 ${stamp}`,
      target_type: "probe.probe",
      query: "",
      columns: "",
      sort: "",
    });
    ids.push(["core.saved_search", search]);
    await page.goto(`/types/probe.probe?search=${search}`);
    await page.getByRole("button", { name: `Gespeicherte Suche: UI62 ${stamp}` }).click();
    await page.getByRole("menuitem", { name: "Diese Suche löschen" }).click();
    await expect(page).not.toHaveURL(/search=/);
    await undo(page, `/api/v1/records/core.saved_search/${search}`);
  } finally {
    for (const [typeName, id] of ids) {
      await page.request.delete(`/api/v1/records/${typeName}/${id}`, { headers });
      await page.request.post(`/api/v1/records/${typeName}/${id}:purge`, { headers });
    }
  }
});

// The Prüfweg of UI63: two records and a dashboard in the trash, Verwaltung › Papierkorb shows
// them with type, title, time and person; the type narrows; a restore gives "Öffnen"; purging
// asks with the number and the record is gone (404); axe without findings, also with the
// question open.
test("the trash page restores and purges", async ({ page }) => {
  await signIn(page);
  const stamp = `UI63 ${Date.now()}`;
  const ids: [string, string][] = [];
  try {
    for (const [typeName, data] of [
      ["probe.probe", { title: `${stamp} eins` }],
      ["probe.probe", { title: `${stamp} zwei` }],
      ["core.dashboard", { name: `${stamp} Tafel` }],
    ] as const) {
      const id = await create(page, typeName, data);
      ids.push([typeName, id]);
      expect(
        (await page.request.delete(`/api/v1/records/${typeName}/${id}`, { headers })).status(),
      ).toBe(204);
    }
    await page.goto("/");
    await page.getByRole("link", { name: "Papierkorb" }).click();
    await expect(page.getByRole("heading", { name: "Papierkorb", level: 1 })).toBeVisible();
    await expect(page.getByText(/bleibt \d+ Tage hier/)).toBeVisible();
    await page.getByLabel("Titel enthält").fill(stamp);
    const rows = page.getByRole("row").filter({ hasText: stamp });
    await expect(rows).toHaveCount(3);
    await expect(rows.first()).toContainText("Dashboard");
    const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
    expect(results.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

    const typeField = page.getByRole("combobox", { name: "Typ" });
    await pick(typeField, "core.dashboard");
    await expect(rows).toHaveCount(1);
    await pick(typeField, "");
    await expect(rows).toHaveCount(3);

    await page.getByRole("checkbox", { name: `„${stamp} eins“ auswählen` }).click();
    await page.getByRole("button", { name: "Wiederherstellen" }).click();
    const note = page
      .locator('[role="status"], [role="alert"]')
      .filter({ hasText: "1 wiederhergestellt" });
    await expect(note.getByRole("button", { name: "Öffnen" })).toBeVisible();
    await expect(rows).toHaveCount(2);

    await page.getByRole("checkbox", { name: `„${stamp} zwei“ auswählen` }).click();
    await page.getByRole("button", { name: "Endgültig löschen" }).click();
    const ask = page.getByRole("alertdialog", { name: "1 Eintrag endgültig löschen?" });
    await expect(ask).toBeVisible();
    const withAsk = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
    expect(withAsk.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
    await ask.getByRole("button", { name: "Endgültig löschen" }).click();
    await expect(page.getByText("1 endgültig gelöscht")).toBeVisible();
    await expect(rows).toHaveCount(1);
    expect(
      (await page.request.get(`/api/v1/records/probe.probe/${ids[1]![1]}`, { headers })).status(),
    ).toBe(404);
  } finally {
    for (const [typeName, id] of ids) {
      await page.request.delete(`/api/v1/records/${typeName}/${id}`, { headers });
      await page.request.post(`/api/v1/records/${typeName}/${id}:purge`, { headers });
    }
  }
});
