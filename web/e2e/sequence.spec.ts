// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of PL49 in a real browser, the step the command line handed on (UI-Plan E36):
// a new record gets its running number; Strg K and the number with its prefix find it, Enter
// opens it, and the record page shows the number read-only.
import { expect, test } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };

test("the command line finds a record by its running number", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);

  const created = await page.request.post("/api/v1/records/probe.probe", {
    headers,
    data: { title: `PL49 ${Date.now()}`, priority: 3 },
  });
  expect(created.status()).toBe(201);
  const rec = (await created.json()) as { id: string; number: number; title: string };
  expect(rec.number).toBeGreaterThan(0);

  await page.goto("/types/probe.probe");
  await expect(page.getByRole("button", { name: /Suchen oder Befehl/ })).toBeVisible();
  await page.keyboard.press("Control+K");
  await page.keyboard.type(`PRB-${rec.number}`);
  const hit = page
    .getByRole("dialog", { name: "Befehlszeile" })
    .getByRole("option", { name: new RegExp(rec.title) });
  await expect(hit).toBeVisible();
  await hit.click();
  await expect(page).toHaveURL(new RegExp(`/types/probe.probe/${rec.id}$`));
  const number = page.getByLabel("Nummer", { exact: true });
  await expect(number).toHaveValue(`PRB-${rec.number}`);
  await expect(number).toHaveAttribute("readonly", "");
});
