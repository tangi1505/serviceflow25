// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI42 (UI plan E53) in a real browser: a tile saved with a symbol chosen in the
// grid shows it in its box over the name in the portal; the grid and the start page without
// findings of axe. The tile is deleted again, so the other tests see the start page as before.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

async function axe(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`),
  ).toEqual([]);
}

test("a tile with a symbol from the grid shows it in the portal", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);

  const name = `UI42 ${Date.now()}`;
  await page.goto("/types/core.portal_tile/new");
  await page.getByLabel(/^Titel/).fill(name);
  await page.getByLabel(/^Führt zu/).selectOption("link");
  await page.getByLabel(/^Adresse/).fill("https://intranet.example/telefon");
  const grid = page.getByRole("group", { name: "Symbol" });
  await expect(grid.getByRole("radio", { name: "Ohne" })).toBeChecked();
  await grid.getByTitle("Telefon").click();
  await expect(grid.getByRole("radio", { name: "Telefon" })).toBeChecked();
  await axe(page);
  await page.getByRole("button", { name: "Anlegen" }).click();
  await expect(page).toHaveURL(/\/types\/core\.portal_tile\/[0-9a-f-]{36}$/);
  const id = page.url().split("/").pop()!;

  try {
    await page.goto("/portal");
    const tiles = page.getByRole("region", { name: "Was möchten Sie tun?" });
    const tile = tiles.getByRole("link", { name: new RegExp(name) });
    await expect(tile).toBeVisible();
    await expect(tile.locator("[data-icon]")).toHaveAttribute("data-icon", "phone");
    await axe(page);
  } finally {
    await page.request.delete(`/api/v1/records/core.portal_tile/${id}`, {
      headers: { "X-Requested-With": "itsm" },
    });
  }
});
