// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI11 in a real browser: the administrator builds a form with three sections
// by keyboard alone, including dragging a section with the keyboard (dnd-kit), and offers it
// in the portal; the user of the portal fills it in and gets the record with the free answers
// in its timeline; axe without findings on both pages.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";

async function signIn(page: Page, user: string, password: string, landing: RegExp) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill(user);
  await page.getByLabel("Kennwort").fill(password);
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(landing);
}

// typeInto focuses a field and types into it, as a keyboard does.
async function typeInto(field: Locator, text: string) {
  await field.focus();
  await field.press("ControlOrMeta+a");
  await field.pressSequentially(text);
}

// pick chooses an option of a closed select by typing its label (type-ahead).
async function pick(select: Locator, label: string) {
  await select.focus();
  await select.pressSequentially(label);
}

async function press(button: Locator, key = "Enter") {
  await button.focus();
  await button.press(key);
}

test("a form built by keyboard is filled in in the portal", async ({ page }) => {
  const name = `UI11 ${Date.now()}`;
  await signIn(page, "load", "Messlauf-Pw7!", /\/$/);
  await page.goto("/types/core.form/new");
  await typeInto(page.getByLabel(/^Name/), name);
  await typeInto(page.getByLabel(/^Legt an/), "probe.probe");
  await page.getByLabel("Im Portal angeboten").focus();
  await page.keyboard.press("Space");
  const builder = page.getByRole("group", { name: "Aufbau" });

  // Section 1: the title, asked in words of its own.
  await typeInto(
    builder.getByRole("region", { name: "Abschnitt 1" }).getByLabel("Titel des Abschnitts"),
    "Anliegen",
  );
  const s1 = builder.getByRole("region", { name: "Anliegen" });
  await pick(s1.getByLabel("Neue Frage"), "Titel");
  await press(s1.getByRole("button", { name: "Frage hinzufügen" }));
  await typeInto(s1.getByLabel("Frage (leer: Name des Felds)"), "Worum geht es?");

  // Section 2: Etage, a choice.
  await press(builder.getByRole("button", { name: "Abschnitt hinzufügen" }));
  await typeInto(
    builder.getByRole("region", { name: "Abschnitt 2" }).getByLabel("Titel des Abschnitts"),
    "Etage",
  );
  const s2 = builder.getByRole("region", { name: "Etage" });
  await pick(s2.getByLabel("Neue Frage"), "Auswahl");
  await press(s2.getByRole("button", { name: "Frage hinzufügen" }));
  await typeInto(s2.getByLabel("Frage", { exact: true }), "Welche Etage?");
  const options = s2.getByLabel("Antworten, je Zeile eine");
  await options.focus();
  await options.pressSequentially("Erdgeschoss");
  await options.press("Enter");
  await options.pressSequentially("1. OG");

  // Section 3: Ort, a required text.
  await press(builder.getByRole("button", { name: "Abschnitt hinzufügen" }));
  await typeInto(
    builder.getByRole("region", { name: "Abschnitt 3" }).getByLabel("Titel des Abschnitts"),
    "Ort",
  );
  const s3 = builder.getByRole("region", { name: "Ort" });
  await pick(s3.getByLabel("Neue Frage"), "Text");
  await press(s3.getByRole("button", { name: "Frage hinzufügen" }));
  await typeInto(s3.getByLabel("Frage", { exact: true }), "Raum");
  await s3.getByLabel("Pflicht").focus();
  await page.keyboard.press("Space");

  // Ort before Etage, dragged with the keyboard: space picks up, arrow up, space drops. Each
  // step waits for what dnd-kit announces: it measures the sections after picking up, and an
  // arrow before that moves nothing.
  await builder.getByRole("button", { name: "„Ort“ verschieben" }).focus();
  await page.keyboard.press("Space");
  await expect(page.getByText("„Ort“ aufgenommen.")).toBeAttached();
  // The announcement comes before the measuring is done: two frames later it is (PL52; an
  // arrow too early moved nothing now and then).
  await page.evaluate(
    () => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))),
  );
  await page.keyboard.press("ArrowUp");
  await expect(page.getByText("„Ort“ bei „Etage“.")).toBeAttached();
  await page.keyboard.press("Space");
  await expect(page.getByText("„Ort“ bei „Etage“ abgelegt.")).toBeAttached();
  await expect
    .poll(async () =>
      builder.getByRole("region").evaluateAll((rs) => rs.map((r) => r.getAttribute("aria-label"))),
    )
    .toEqual(["Anliegen", "Ort", "Etage"]);
  const built = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(built.violations.map((v) => v.id)).toEqual([]);
  await press(page.getByRole("button", { name: "Anlegen" }));
  await expect(page.getByRole("heading", { name })).toBeVisible();

  // The user of the portal (role e2e_portal: creates probe.probe) fills it in.
  await page.getByRole("button", { name: "Abmelden" }).click();
  await signIn(page, "nutzerin", "Nur-Portal10!", /\/portal$/);
  await page.getByRole("region", { name: "Formulare" }).getByRole("link", { name }).click();
  await expect(page.getByRole("heading", { name })).toBeVisible();
  const title = `Drucker ${Date.now()}`;
  await page
    .getByRole("group", { name: "Anliegen" })
    .getByLabel(/Worum geht es\?/)
    .fill(title);
  await page.getByRole("group", { name: "Ort" }).getByLabel(/Raum/).fill("2.14");
  await page
    .getByRole("group", { name: "Etage" })
    .getByLabel("Welche Etage?")
    .selectOption({ label: "1. OG" });
  const filled = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(filled.violations.map((v) => v.id)).toEqual([]);
  await page.getByRole("button", { name: "Absenden" }).click();

  await expect(page.getByRole("heading", { name: title })).toBeVisible();
  await expect(page).toHaveURL(/\/portal\/probe\.probe\//);
  const timeline = page.getByRole("region", { name: "Zeitleiste" });
  await expect(timeline).toContainText("Raum: 2.14");
  await expect(timeline).toContainText("Welche Etage?: 1. OG");
});
