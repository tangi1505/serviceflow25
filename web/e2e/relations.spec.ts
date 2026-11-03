// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The relations tab in a real browser (SD4, UI-Plan E38): the first tab of a record page; a
// record found by name is linked and shows on both sides, with axe finding nothing. The CI
// instance has no service desk packages, so two probes stand in for incident and element.
import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };

test("links two records in the relations tab and shows it on both", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);

  const stamp = Date.now();
  const make = async (title: string) => {
    const r = await page.request.post("/api/v1/records/probe.probe", {
      headers,
      data: { title, priority: 3 },
    });
    expect(r.status()).toBe(201);
    return (await r.json()) as { id: string; number: number };
  };
  const incident = await make(`SD4 Fachverfahren ${stamp}`);
  const element = await make(`SD4 Server ${stamp}`);

  await page.goto(`/types/probe.probe/${incident.id}`);
  const tabs = page.getByRole("tablist", { name: "Bereiche des Datensatzes" });
  await expect(tabs.getByRole("tab").first()).toHaveAccessibleName("Bezüge");
  const panel = page.getByRole("tabpanel", { name: "Bezüge" });
  await expect(panel.getByText("Noch keine Bezüge.")).toBeVisible();
  await panel.getByLabel("Art").selectOption({ label: "verknüpft mit" });
  await panel.getByRole("combobox", { name: "Datensatz" }).fill(`Server ${stamp}`);
  await page.getByRole("option", { name: new RegExp(`SD4 Server ${stamp}`) }).click();
  await panel.getByRole("button", { name: "Verknüpfen" }).click();
  const link = panel.getByRole("link", { name: `PRB-${element.number} · SD4 Server ${stamp}` });
  await expect(link).toBeVisible();
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(results.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  // The other side shows it too.
  await link.click();
  await expect(page).toHaveURL(new RegExp(`/types/probe.probe/${element.id}$`));
  await expect(
    page
      .getByRole("tabpanel", { name: "Bezüge" })
      .getByRole("link", { name: `PRB-${incident.number} · SD4 Fachverfahren ${stamp}` }),
  ).toBeVisible();
});
