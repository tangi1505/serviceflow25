// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI29 in a real browser: the font is IBM Plex from the tool itself, the main
// button graphite, the focus ring petrol; no request leaves the tool; the theme "probe" still
// changes the components; "Dunkel" in the settings turns every page dark, also after signing in
// again; axe with contrast finds nothing on the main pages, light and dark. The account
// gestalter (made by scripts/ci.sh) goes back to "like the system" at the end.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const who = { user: "gestalter", password: "Dunkel-Hell29!" };
const headers = { "X-Requested-With": "itsm" };

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill(who.user);
  await page.getByLabel("Kennwort").fill(who.password);
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

async function axe(page: Page, where: string) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map(
      (v) => `${where} ${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`,
    ),
  ).toEqual([]);
}

// pages are the main pages the design reaches; each is checked by axe in both modes.
async function pages(page: Page, id: string, mode: string) {
  for (const [path, heading] of [
    ["/types/probe.probe", "Probe"],
    [`/types/probe.probe/${id}`, "UI29"],
    ["/types/probe.probe/workflow", "Workflow: Probe"],
    ["/roles", "Rollen"],
    ["/settings", "Meine Einstellungen"],
  ] as const) {
    await page.goto(path);
    await expect(page.getByRole("heading", { name: heading, exact: false }).first()).toBeVisible();
    await axe(page, `${mode} ${path}`);
  }
}

test.afterAll(async ({ browser }) => {
  const page = await (await browser.newContext()).newPage();
  await signIn(page);
  await page.request.patch("/api/v1/me/settings", {
    headers,
    data: { color_scheme: null, theme: null },
  });
});

test("IBM Plex, graphite and petrol from the tool itself; dark after signing in again", async ({
  page,
}) => {
  const outside: string[] = [];
  page.on("request", (r) => {
    const url = new URL(r.url());
    if (!["127.0.0.1", "localhost"].includes(url.hostname) && url.protocol !== "data:") {
      outside.push(r.url());
    }
  });
  await signIn(page);
  const created = await page.request.post("/api/v1/records/probe.probe", {
    headers,
    data: { title: "UI29 Gestaltung", priority: 1 },
  });
  const id = ((await created.json()) as { id: string }).id;

  // The font of the tool, loaded from the tool.
  await page.goto("/types/probe.probe");
  await expect(page.getByRole("heading", { name: "Probe", exact: true })).toBeVisible();
  expect(await page.evaluate(() => getComputedStyle(document.body).fontFamily)).toContain(
    "IBM Plex Sans",
  );
  expect(
    await page.evaluate(async () => {
      // Mono is only loaded once a page uses it; asking for it loads it from the tool.
      const sans = await document.fonts.load('14px "IBM Plex Sans"');
      const mono = await document.fonts.load('13px "IBM Plex Mono"');
      return sans.length > 0 && mono.length > 0;
    }),
  ).toBe(true);
  // The main button graphite; the focus ring petrol.
  // The first button Neu is the list's own; a filter may offer the state Neu as well.
  const create = page.getByRole("button", { name: "Neu", exact: true }).first();
  expect(await create.evaluate((b) => getComputedStyle(b).backgroundColor)).toBe("rgb(21, 23, 26)");
  await create.focus();
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Tab");
  expect(await create.evaluate((b) => getComputedStyle(b).outlineColor)).toBe("rgb(11, 110, 128)");
  // State and priority as marks.
  await expect(page.getByText("P1").first()).toBeVisible();

  // The theme "probe" still reaches the components; since UI35 it is chosen in Meine
  // Einstellungen only, so the test sets it on the page directly.
  await page.evaluate(() => {
    document.documentElement.dataset.theme = "probe";
  });
  expect(await create.evaluate((b) => getComputedStyle(b).backgroundColor)).toBe(
    "rgb(63, 75, 184)",
  );
  await page.evaluate(() => {
    delete document.documentElement.dataset.theme;
  });

  await pages(page, id, "light");

  // Dark in the settings: at once, on every page, and after signing in again.
  await page.goto("/settings");
  await page.getByLabel("Darstellung").selectOption("dark");
  await page.getByRole("button", { name: "Speichern" }).click();
  await expect(page.getByText("Gespeichert.")).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-color-scheme", "dark");
  expect(await page.evaluate(() => getComputedStyle(document.body).backgroundColor)).toBe(
    "rgb(17, 19, 21)",
  );
  // Signing out sits in the account menu of the header (UI35).
  await page.getByRole("button", { name: /^Konto von / }).click();
  await page.getByRole("button", { name: "Abmelden" }).click();
  await signIn(page);
  await expect(page.locator("html")).toHaveAttribute("data-color-scheme", "dark");
  await pages(page, id, "dark");

  expect(outside).toEqual([]);
});
