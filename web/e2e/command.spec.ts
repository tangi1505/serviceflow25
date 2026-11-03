// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI31 in a real browser, with the keyboard only: Strg K, a word of the title →
// the record is the first option, Enter opens it; Strg K, "neu" → "Neu: Probe", Enter leads to
// the form; empty, the command line lists the record opened last; Esc closes it. axe without
// findings with the command line open. The number of a record follows with PL49.
import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };

test("the command line finds, opens and creates with the keyboard only", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
  const word = `Befehlszeile${Date.now()}`;
  const created = await page.request.post("/api/v1/records/probe.probe", {
    headers,
    data: { title: `UI31 ${word}`, priority: 3 },
  });
  expect(created.status()).toBe(201);
  const id = ((await created.json()) as { id: string }).id;

  await page.goto("/types/probe.probe");
  await expect(page.getByRole("button", { name: /Suchen oder Befehl/ })).toBeVisible();
  await page.keyboard.press("Control+K");
  const dialog = page.getByRole("dialog", { name: "Befehlszeile" });
  const input = dialog.getByRole("combobox", { name: "Befehl oder Suche" });
  await expect(input).toBeFocused();
  await page.keyboard.type(word);
  const hit = dialog
    .getByRole("group", { name: "Datensätze" })
    .getByRole("option", { name: new RegExp(word) });
  await expect(hit).toHaveAttribute("aria-selected", "true");
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`),
  ).toEqual([]);
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(new RegExp(`/types/probe.probe/${id}$`));
  await expect(page.getByRole("heading", { level: 1, name: `UI31 ${word}` })).toBeVisible();

  // A new record from the command line.
  await page.keyboard.press("Control+K");
  await page.keyboard.type("neu");
  await expect(dialog.getByRole("option", { name: "Neu: Probe", exact: true })).toBeVisible();
  await page.keyboard.type(" probe");
  await expect(dialog.getByRole("option", { name: "Neu: Probe", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/types\/probe\.probe\/new$/);

  // Empty, it lists the record opened last; Esc closes it.
  await page.keyboard.press("Control+K");
  await expect(
    dialog
      .getByRole("group", { name: "Zuletzt geöffnet" })
      .getByRole("option", { name: new RegExp(word) }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
});
