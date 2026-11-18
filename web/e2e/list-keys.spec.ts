// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// UI47 to UI53 in a real browser: the head with "Anpassen", the bar in one row, the filter row
// by keyboard with a period kept as a time word, "/" and "?" and Strg B, the list with arrows,
// Enter and Umschalt Enter; axe without findings with menu, suggestions and dialog open.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

async function noViolations(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`),
  ).toEqual([]);
}

test.use({ viewport: { width: 1440, height: 900 } });

test("head, bar, filter row and keys of the list", async ({ page }) => {
  await signIn(page);
  const stamp = `UI50 ${Date.now()}`;
  for (const n of [1, 2, 3]) {
    const r = await page.request.post("/api/v1/records/probe.probe", {
      headers,
      data: { title: `${stamp} ${n}`, priority: 2 },
    });
    expect(r.status()).toBe(201);
  }
  await page.goto("/types/probe.probe");

  // The head (UI47): "Anpassen" opens a menu with two entries, Esc closes it.
  await page.locator("main").getByRole("button", { name: "Anpassen" }).click();
  const menu = page.getByRole("menu", { name: "Anpassen" });
  await expect(menu.getByRole("menuitem")).toHaveCount(2);
  await noViolations(page);
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(page.locator("main").getByRole("button", { name: "Anpassen" })).toBeFocused();

  // The bar (UI48): every control 32 px high.
  for (const name of [/Gespeicherte Suche/, /^Ansicht$/, /^Exportieren$/]) {
    expect((await page.locator("main").getByRole("button", { name }).boundingBox())!.height).toBe(
      32,
    );
  }

  // "/" jumps into the filter row (UI52); "titel", Enter, the stamp, Enter (UI50).
  await page.locator("main").click({ position: { x: 5, y: 5 } });
  await page.keyboard.press("/");
  const filter = page.getByRole("combobox", { name: "Filter setzen" });
  await expect(filter).toBeFocused();
  await page.keyboard.type("titel");
  await page.keyboard.press("Enter");
  await page.keyboard.type(stamp);
  await expect(page.getByRole("listbox", { name: "Wert" })).toBeVisible();
  await noViolations(page);
  await page.keyboard.press("Enter");
  await expect(page.getByText("3 Datensätze")).toBeVisible();

  // A period (UI51): "angelegt", Enter, "letzte 5 t", Enter; the query holds the time word.
  const asked = page.waitForRequest(
    (r) => r.url().endsWith("probe.probe:query") && (r.postData() ?? "").includes("@now-5d"),
  );
  await page.keyboard.type("angel");
  await page.keyboard.press("Enter");
  await page.keyboard.type("letzte 5 t");
  await page.keyboard.press("Enter");
  await asked;
  await expect(page.getByRole("button", { name: /Angelegt:\s*letzte 5 Tage/ })).toBeVisible();
  await expect(page.getByText("3 Datensätze")).toBeVisible();
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");

  // The list by keyboard (UI53): arrow down, Enter shows the preview, Umschalt Enter opens.
  const rows = page.locator("tbody tr");
  await rows.first().focus();
  await page.keyboard.press("ArrowDown");
  await expect(rows.nth(1)).toBeFocused();
  const second = (await rows.nth(1).textContent()) ?? "";
  await page.keyboard.press("Enter");
  const preview = page.getByRole("complementary", { name: "Vorschau" });
  await expect(preview).toBeVisible();
  expect(second).toContain(stamp);
  await page.keyboard.press("Escape");
  await expect(preview).toHaveCount(0);

  // "?" shows the dialog (UI52); Esc closes it and the focus is back on the row.
  await rows.nth(1).focus();
  await page.keyboard.press("Shift+?");
  const dialog = page.getByRole("dialog", { name: "Tastenkürzel" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText("Vorgang öffnen")).toBeVisible();
  await noViolations(page);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(rows.nth(1)).toBeFocused();

  // Strg B folds the side bar and back (UI52).
  await page.keyboard.press("Control+b");
  await expect(page.getByRole("button", { name: "Leiste ausklappen" })).toBeVisible();
  await page.keyboard.press("Control+b");
  await expect(page.getByRole("button", { name: "Leiste einklappen" })).toBeVisible();

  await rows.nth(1).focus();
  await page.keyboard.press("Shift+Enter");
  await expect(page).toHaveURL(/\/types\/probe\.probe\/[0-9a-f-]{36}$/);
});
