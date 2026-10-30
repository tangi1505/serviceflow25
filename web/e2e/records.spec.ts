// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI2 in a real browser against the server: the list of probe.probe, create
// and edit in the form, a conflict between two windows (E29) with both values shown, axe
// without findings, and the time of a page change as a first reading for LZ9 (≤ 500 ms).
// UI3: saving stays on the record page; the timeline shows the changes and new entries,
// newest first, a private one marked. UI28: a role chosen by name with the picker.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const user = process.env["ITSM_E2E_USER"] ?? "load";
const password = process.env["ITSM_E2E_PASSWORD"] ?? "Messlauf-Pw7!";

async function expectNoViolations(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map(
      (v) =>
        `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(" ") + " " + JSON.stringify(n.any[0]?.data)).join(", ")})`,
    ),
  ).toEqual([]);
}

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill(user);
  await page.getByLabel("Kennwort").fill(password);
  await page.getByRole("button", { name: "Anmelden" }).click();
  await expect(page).toHaveURL(/\/$/);
}

test("list, create, edit and resolve a conflict", async ({ page, context }) => {
  await signIn(page);
  const title = `E2E ${Date.now()}`;

  // Page change into the list: from the click to the rows, a first reading for LZ9.
  const started = Date.now();
  await page.getByRole("link", { name: "Probe" }).click();
  await expect(page.getByRole("heading", { name: "Probe" })).toBeVisible();
  await expect(page.getByText(/Datensätze$/)).toBeVisible();
  const elapsed = Date.now() - started;
  console.log(`LZ9 Seitenwechsel zur Liste: ${elapsed} ms`);
  expect(elapsed).toBeLessThan(500);
  await expectNoViolations(page);

  // Create.
  // Exact: "Als neue Suche speichern" (UI4) contains "neu" too.
  await page.getByRole("button", { name: "Neu", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Neuer Datensatz" })).toBeVisible();
  await expectNoViolations(page);
  await page.getByLabel(/Titel/).fill(title);
  // Exact: "Wartegrund" (WF11) contains "art" too.
  await page.getByLabel("Art", { exact: true }).selectOption("fault");
  await page.getByLabel("Priorität", { exact: true }).fill("2");
  await page.getByRole("button", { name: "Anlegen" }).click();
  await expect(page.getByRole("heading", { name: title })).toBeVisible();
  // Exact: the blocks of the timeline name their version too (UI3).
  await expect(page.getByText("Version 1", { exact: true })).toBeVisible();
  const recordUrl = page.url();

  // The state model (WF1): new → start → in progress; finish is disabled before that.
  const stateBox = page.getByRole("region", { name: "Status" });
  await expect(stateBox).toContainText("Neu");
  await expect(stateBox.getByRole("button", { name: "Erledigen" })).toBeDisabled();
  await stateBox.getByRole("button", { name: "Beginnen" }).click();
  await expect(stateBox).toContainText("In Arbeit");
  await expect(page.getByText("Version 2", { exact: true })).toBeVisible();
  await expectNoViolations(page);

  // A second window edits the same record and saves first.
  const other = await context.newPage();
  await other.goto(recordUrl);
  await expect(other.getByLabel(/Titel/)).toHaveValue(title);
  await other.getByLabel(/Titel/).fill(`${title} (Fenster 2)`);
  await other.getByRole("button", { name: "Speichern" }).click();
  // Saving stays on the page (UI3; until then it went back to the list).
  await expect(other.getByText("Gespeichert.")).toBeVisible();
  await other.close();

  // The first window saves afterwards and sees the conflict with both values.
  await page.getByLabel(/Titel/).fill(`${title} (Fenster 1)`);
  await page.getByRole("button", { name: "Speichern" }).click();
  const alert = page.getByRole("alert");
  await expect(alert).toContainText("Jemand hat diesen Datensatz inzwischen gespeichert.");
  await expect(alert).toContainText(`${title} (Fenster 2)`);
  await expect(alert).toContainText(`${title} (Fenster 1)`);
  await expectNoViolations(page);
  await page.getByRole("button", { name: "Serverstand übernehmen" }).click();
  await expect(page.getByLabel(/Titel/)).toHaveValue(`${title} (Fenster 2)`);

  // Edit after the reload goes through with the new version.
  await page.getByLabel(/Titel/).fill(`${title} (einig)`);
  await page.getByRole("button", { name: "Speichern" }).click();
  await expect(page.getByText("Gespeichert.")).toBeVisible();
  await expect(page.getByText("Version 4", { exact: true })).toBeVisible();

  // The timeline (UI3): the changes so far, then two new entries on top, one private.
  const timeline = page.getByRole("region", { name: "Zeitleiste" });
  await expect(timeline).toContainText(`Titel: ${title} (Fenster 2) → ${title} (einig)`);
  await expect(timeline).toContainText("Status: Neu → In Arbeit");
  await expect(timeline).toContainText("Angelegt");
  // The entry is the editor of UI7, typed like a person types.
  const editor = timeline.getByRole("textbox", { name: "Neuer Eintrag", exact: true });
  await editor.pressSequentially("Drucker steht im 3. Stock.");
  await timeline.getByRole("button", { name: "Eintragen" }).click();
  await expect(timeline.locator(":scope > ol > li").first()).toContainText(
    "Drucker steht im 3. Stock.",
  );
  await editor.fill("Intern: Ersatzteil bestellt.");
  await timeline.getByLabel("Privat (nur für Berechtigte)").check();
  await timeline.getByRole("button", { name: "Eintragen" }).click();
  const newest = timeline.locator(":scope > ol > li").first();
  await expect(newest).toContainText("Intern: Ersatzteil bestellt.");
  await expect(newest).toContainText("Privat");
  // UI7: formatted text with an image chosen in the editor; the image becomes a file at the
  // record and stands in the text.
  await editor.click();
  await page.keyboard.type("Siehe ");
  await timeline.getByRole("button", { name: "Fett" }).click();
  await expect(editor).toBeFocused();
  await page.keyboard.type("Foto");
  await timeline.getByLabel("Bild einfügen").setInputFiles({
    name: "router.png",
    mimeType: "image/png",
    buffer: Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
      "base64",
    ),
  });
  await expect(editor.locator("img")).toHaveCount(1);
  await timeline.getByRole("button", { name: "Eintragen" }).click();
  // The entry with the image; the history row "Datei angehängt" of the same second may stand
  // above it.
  const withImage = timeline.locator(":scope > ol > li", { has: page.locator("img") }).first();
  await expect(withImage.locator("strong")).toHaveText("Foto");
  const src = await withImage.locator("img").getAttribute("src");
  expect(src).toMatch(/^\/api\/v1\/files\/[0-9a-f-]{36}$/);
  const documents = await page.request.get(
    new URL(recordUrl).pathname.replace("/types/", "/api/v1/records/") + "/documents",
  );
  expect(await documents.text()).toContain(src!.split("/").pop()!);
  await expectNoViolations(page);

  await timeline.getByRole("button", { name: "Nur Einträge" }).click();
  await expect(timeline).not.toContainText("Angelegt");
  await expect(timeline.locator(":scope > ol > li")).toHaveCount(3);
  // The entries left the record's version alone.
  await expect(page.getByText("Version 4", { exact: true })).toBeVisible();
  await expectNoViolations(page);

  // UI28: the requester chosen by name through the full-text search, saved with the fields;
  // the sidebar and the timeline name the person.
  const surname = `Picker${Date.now()}`;
  const person = await page.request.post("/api/v1/records/core.person", {
    headers: { "X-Requested-With": "itsm" },
    data: { first_name: "Erika", last_name: surname },
  });
  expect(person.status()).toBe(201);
  const requester = page.getByRole("combobox", { name: "Anforderer" });
  await requester.fill(surname.slice(0, 10));
  await page.getByRole("option", { name: new RegExp(surname) }).click();
  await expect(page.getByRole("group", { name: "Rollen am Datensatz" })).toContainText(
    `Erika ${surname}`,
  );
  await expectNoViolations(page);
  await page.getByRole("button", { name: "Speichern" }).click();
  await expect(page.getByText("Version 5", { exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Rollen" })).toContainText(`Erika ${surname}`);
  await timeline.getByRole("button", { name: "Alles" }).click();
  await expect(timeline).toContainText(`Anforderer: – → Erika ${surname}`);
});

// UI4: a filter narrows the list through the JSON query, the view is saved as a search and a
// link opens it again; sharing with another person and their rights are proven over HTTP
// (TestSavedSearchesOverHTTP).
test("filter the list and save the view as a search", async ({ page }) => {
  await signIn(page);
  const stamp = `UI4 ${Date.now()}`;
  for (const kind of ["fault", "question"]) {
    const r = await page.request.post("/api/v1/records/probe.probe", {
      headers: { "X-Requested-With": "itsm" },
      data: { title: `${stamp} ${kind}`, kind },
    });
    expect(r.status()).toBe(201);
  }
  await page.goto("/types/probe.probe");
  await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
  const first = page.getByRole("listitem", { name: "Bedingung 1" });
  await first.getByRole("textbox", { name: "Wert" }).fill(stamp);
  await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
  const second = page.getByRole("listitem", { name: "Bedingung 2" });
  await second.getByRole("combobox", { name: "Feld" }).selectOption({ label: "Art" });
  await second.getByRole("combobox", { name: "Wert" }).selectOption("fault");
  await expect(page.getByText("1 Datensätze")).toBeVisible();
  await expect(page.getByRole("grid")).toContainText(`${stamp} fault`);
  await expectNoViolations(page);

  await page.getByRole("button", { name: "Als neue Suche speichern" }).click();
  await page.getByLabel("Name der Suche").fill(stamp);
  await page.getByRole("button", { name: "Speichern", exact: true }).click();
  await expect(page).toHaveURL(/\?search=/);
  const link = page.url();
  await page.goto("/types/probe.probe");
  await expect(page.getByText("1 Datensätze")).not.toBeVisible();
  await page.goto(link);
  await expect(page.getByRole("heading", { name: new RegExp(stamp) })).toBeVisible();
  await expect(page.getByText("1 Datensätze")).toBeVisible();
  await expect(
    page.getByRole("listitem", { name: "Bedingung 2" }).getByRole("combobox", { name: "Wert" }),
  ).toHaveValue("fault");

  // UI5: the export of this view holds exactly its row, the browser saves it as a file.
  await page.getByLabel("Format des Exports").selectOption("csv");
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Exportieren" }).click();
  const file = await download;
  expect(file.suggestedFilename()).toMatch(/^probe-probe-.*\.csv$/);
  const content = await (await file.createReadStream()).toArray();
  const text = Buffer.concat(content).toString("utf-8");
  expect(text.split("\r\n").filter(Boolean)).toHaveLength(2);
  expect(text).toContain(`${stamp} fault`);
  expect(text).toContain("Störung");
  await expectNoViolations(page);
});

// UI6: 500 records chosen in the list, a field set on all of them in one call to :batch, the
// result per row shown.
test("set a field on 500 chosen records in one call", async ({ page }) => {
  test.setTimeout(120_000);
  await signIn(page);
  const stamp = `UI6 ${Date.now()}`;
  const rows = Array.from({ length: 500 }, (_, i) => ({
    external_key: `${stamp}-${i}`,
    title: `${stamp} ${String(i).padStart(3, "0")}`,
    priority: 3,
  }));
  const bulk = await page.request.post("/api/v1/records/probe.probe:bulk", {
    headers: { "X-Requested-With": "itsm" },
    data: { source: "e2e-ui6", rows },
  });
  expect(bulk.status()).toBe(200);

  await page.goto("/types/probe.probe");
  await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
  await page
    .getByRole("listitem", { name: "Bedingung 1" })
    .getByRole("textbox", { name: "Wert" })
    .fill(stamp);
  await expect(page.getByText(/^100\+? Datensätze$/)).toBeVisible();
  // Load every page: scroll the list to its end until all 500 are there.
  const grid = page.getByRole("grid").locator("..");
  for (let i = 0; i < 20 && !(await page.getByText("500 Datensätze").isVisible()); i++) {
    await grid.evaluate((el) => el.scrollTo(0, el.scrollHeight));
    await page.waitForTimeout(300);
  }
  await expect(page.getByText("500 Datensätze")).toBeVisible();

  const batches: string[] = [];
  page.on("request", (r) => {
    if (r.url().endsWith(":batch")) {
      batches.push(r.url());
    }
  });
  await page.getByRole("checkbox", { name: "Alle geladenen auswählen" }).check();
  const bar = page.getByRole("region", { name: "500 gewählt" });
  await bar.getByRole("combobox", { name: "Feld" }).selectOption({ label: "Priorität" });
  await bar.getByRole("spinbutton", { name: "Neuer Wert" }).fill("5");
  await expectNoViolations(page);
  await bar.getByRole("button", { name: "Auf 500 anwenden" }).click();
  await expect(bar.getByText("500 geändert")).toBeVisible({ timeout: 60_000 });
  expect(batches).toHaveLength(1);
});
