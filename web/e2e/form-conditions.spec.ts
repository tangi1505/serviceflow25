// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI12 in a real browser: a form whose question B shows only when A is "yes",
// filled in by a user of the portal who reads English; the form speaks English, B appears and
// goes with A, and the record's entry is German. The CI gives the account englisch the role
// e2e_portal; the test sets her language itself.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

async function signIn(page: Page, user: string, password: string, landing: RegExp) {
  await page.goto("/login");
  await page.getByLabel(/Benutzername|Username/).fill(user);
  await page.getByLabel(/Kennwort|Password/).fill(password);
  await page.getByRole("button", { name: /^(Anmelden|Sign in)$/ }).click();
  await expect(page).toHaveURL(landing);
}

const csrf = { "X-Requested-With": "itsm" };

test("question B shows only for yes, in English", async ({ page }) => {
  const name = `UI12 ${Date.now()}`;
  const english = `Spare ${Date.now()}`;
  await signIn(page, "load", "Messlauf-Pw7!", /\/$/);
  const definition = {
    title: { en: english },
    sections: [
      {
        id: "s1",
        title: { de: "Anliegen", en: "Request" },
        questions: [
          { id: "q1", field: "title", label: { de: "Worum geht es?", en: "What is it about?" } },
          {
            id: "q2",
            type: "boolean",
            label: { de: "Ersatzgerät nötig?", en: "Spare device needed?" },
          },
          {
            id: "q3",
            type: "text",
            label: { de: "Welches Gerät?", en: "Which device?" },
            required: true,
            show_if: [{ question: "q2", op: "eq", value: "true" }],
          },
        ],
      },
    ],
  };
  const created = await page.request.post("/api/v1/records/core.form", {
    headers: csrf,
    data: {
      name,
      target_type: "probe.probe",
      active: true,
      definition: JSON.stringify(definition),
    },
  });
  expect(created.status()).toBe(201);
  await page.getByRole("button", { name: "Abmelden" }).click();

  await signIn(page, "englisch", "Portal-Sprache12!", /\/portal$/);
  expect(
    (
      await page.request.patch("/api/v1/me/settings", { headers: csrf, data: { language: "en" } })
    ).status(),
  ).toBe(200);
  await page.goto("/portal");
  await page.getByRole("region", { name: "Forms" }).getByRole("link", { name: english }).click();
  const request = page.getByRole("group", { name: "Request" });
  await expect(request.getByLabel("Which device?")).toHaveCount(0);
  await request.getByLabel("Spare device needed?").check();
  await expect(request.getByLabel(/Which device\?/)).toBeVisible();
  await request.getByLabel(/What is it about\?/).fill(`Laptop ${Date.now()}`);
  // Required once shown.
  await page.getByRole("button", { name: "Send" }).click();
  await expect(request.getByText("Please fill in.")).toBeVisible();
  await request.getByLabel(/Which device\?/).fill("Laptop 14");
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(results.violations.map((v) => v.id)).toEqual([]);
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page).toHaveURL(/\/portal\/probe\.probe\//);
  const timeline = page.getByRole("region", { name: "Timeline" });
  await expect(timeline).toContainText("Ersatzgerät nötig?: ja");
  await expect(timeline).toContainText("Welches Gerät?: Laptop 14");
});
