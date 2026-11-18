// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI34 in a real browser: with an SLA on Probe the list shows the column
// "Frist"; sorted by it, the record with the nearest deadline stands first and records without
// one come last; the filter "ist überschritten" leaves out what still runs; axe without
// findings. Yellow and red at the cell itself are shown in Vitest (the CI cannot wait for 45
// minutes). The SLA goes again at the end, so that the other tests see Probe as before.
import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const headers = { "X-Requested-With": "itsm" };

test("the deadline column sorts and filters by the SLA clocks", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);

  const tag = `UI34 ${Date.now()}`;
  const sla = await page.request.post("/api/v1/records/core.sla", {
    headers,
    data: {
      name: `Frist ${tag}`,
      target_type: "probe.probe",
      target_minutes: 60,
      applies: `record.title.startsWith("${tag}")`,
      pause: 'record.state == "waiting"',
      stop: 'record.state in ["resolved", "closed"]',
    },
  });
  expect(sla.status()).toBe(201);
  const slaId = ((await sla.json()) as { id: string }).id;
  try {
    const created = await page.request.post("/api/v1/records/probe.probe", {
      headers,
      data: { title: `${tag} Drucker`, priority: 2 },
    });
    expect(created.status()).toBe(201);
    const rec = (await created.json()) as { sla_due_at: string | null; sla_warn_at: string | null };
    expect(rec.sla_due_at).not.toBeNull();
    expect(rec.sla_warn_at).not.toBeNull();

    await page.goto("/types/probe.probe");
    const header = page.getByRole("columnheader", { name: /Frist/ });
    await expect(header).toBeVisible();
    // Sorted by the deadline, nearest first: the new record leads.
    await header.getByRole("button").click();
    if ((await header.getAttribute("aria-sort")) !== "ascending") {
      await header.getByRole("button").click();
    }
    await expect(header).toHaveAttribute("aria-sort", "ascending");
    const first = page.getByRole("row").nth(1);
    await expect(first).toContainText(`${tag} Drucker`);
    // 60 minutes show as "1 h 0 min" until the first minute is gone.
    await expect(first).toContainText(/Noch (59 min|1 h 0 min)/);
    const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
    expect(
      results.violations.map(
        (v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`,
      ),
    ).toEqual([]);

    // The filter "ist überschritten" leaves the running one out.
    // In the filter row (UI50): the deadline, arrow left to the comparison, "überschritten".
    await page.getByRole("combobox", { name: "Filter setzen" }).click();
    await page.keyboard.type("frist");
    await page.keyboard.press("Enter");
    await page.keyboard.press("ArrowLeft");
    await page.keyboard.type("über");
    await page.keyboard.press("Enter");
    await expect(page.getByRole("button", { name: /Frist\s*ist überschritten/ })).toBeVisible();
    await expect(page.getByRole("row", { name: new RegExp(tag) })).toHaveCount(0);
  } finally {
    await page.request.delete(`/api/v1/records/core.sla/${slaId}`, { headers });
    await page.request.post(`/api/v1/records/core.sla/${slaId}:purge`, { headers });
  }
});
