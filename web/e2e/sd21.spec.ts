// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The walk-through of milestone 1 (SD21, SD-Plan E2): on an instance of its own with only the
// packages of the service desk (scripts/sd21-instance.sh), everything else is configuration an
// administrator does over the API, as a customer would: persons, roles, login rules, the
// approval of a workplace and an article for the knowledge editors. Then three people sign in
// through the identity provider (the rebuilt one of the tests, Pascal's decision 2026-11-20)
// and go through it in the browser:
//   1. Bernd of the service desk approves the article, which is then published.
//   2. Nora reports an incident in the portal.
//   3. Bernd gives it a high impact and urgency, sees its deadline, starts it, takes the article
//      as its solution and resolves it.
//   4. Nora confirms the solution in the portal; the incident is closed.
//   5. Nora orders a new workplace from the catalogue; the request waits for approval.
//   6. Gesa approves it in the portal; the request goes into fulfilment with its tasks.
import AxeBuilder from "@axe-core/playwright";
import {
  expect,
  request as playwrightRequest,
  test,
  type APIRequestContext,
  type Browser,
  type Page,
} from "@playwright/test";
import { pick } from "./pick";

const base = process.env["ITSM_SD21_URL"] ?? "";
const idp = process.env["ITSM_SD21_IDP"] ?? "";
const headers = { "X-Requested-With": "itsm" };

test.skip(!base || !idp, "the instance of SD21 runs only in the CI (scripts/sd21-instance.sh)");
test.use({ baseURL: base, actionTimeout: 15_000 });

type Who = {
  subject: string;
  username: string;
  name: string;
  given_name: string;
  family_name: string;
  email: string;
  groups: string[];
};
const nora: Who = {
  subject: "sd21-nora",
  username: "nora.nutzer",
  name: "Nora Nutzer",
  given_name: "Nora",
  family_name: "Nutzer",
  email: "nora.nutzer@sd21.test",
  groups: ["itsm-portal"],
};
const bernd: Who = {
  subject: "sd21-bernd",
  username: "bernd.bearbeiter",
  name: "Bernd Bearbeiter",
  given_name: "Bernd",
  family_name: "Bearbeiter",
  email: "bernd.bearbeiter@sd21.test",
  groups: ["itsm-servicedesk"],
};
const gesa: Who = {
  subject: "sd21-gesa",
  username: "gesa.genehmigerin",
  name: "Gesa Genehmigerin",
  given_name: "Gesa",
  family_name: "Genehmigerin",
  email: "gesa.genehmigerin@sd21.test",
  groups: ["itsm-portal"],
};

async function noFindings(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`),
  ).toEqual([]);
}

// signIn tells the provider who comes next and signs in through it, in a context of its own.
async function signIn(browser: Browser, who: Who): Promise<Page> {
  const context = await browser.newContext({ baseURL: base, locale: "de-DE" });
  const page = await context.newPage();
  const next = await page.request.post(`${idp}/next`, { data: who });
  expect(next.status()).toBe(204);
  await page.goto("/login");
  await page.getByRole("button", { name: "Anmelden mit Testanbieter" }).click();
  await expect(page.getByRole("button", { name: /^Konto von / })).toBeVisible();
  return page;
}

// The administrator's API: every call must succeed.
async function admin(): Promise<APIRequestContext> {
  const api = await playwrightRequest.newContext({ baseURL: base, extraHTTPHeaders: headers });
  const r = await api.post("/api/v1/auth/login", {
    data: { username: "sd21admin", password: "Durchgang-Pw21!" },
  });
  expect(r.status(), await r.text()).toBe(200);
  return api;
}

async function send(
  api: APIRequestContext,
  method: "POST" | "PUT" | "PATCH",
  url: string,
  data: unknown,
  extra: Record<string, string> = {},
): Promise<Record<string, unknown>> {
  const r = await api.fetch(url, { method, data, headers: extra });
  expect(r.ok(), `${method} ${url}: ${r.status()} ${await r.text()}`).toBe(true);
  const text = await r.text();
  return text ? (JSON.parse(text) as Record<string, unknown>) : {};
}

async function one(api: APIRequestContext, type: string, filter: string) {
  const r = await api.get(`/api/v1/records/${type}?${filter}`);
  expect(r.ok()).toBe(true);
  const body = (await r.json()) as
    { items?: Record<string, unknown>[] } | Record<string, unknown>[];
  const items = Array.isArray(body) ? body : (body.items ?? []);
  expect(items.length, `${type} ${filter}`).toBeGreaterThan(0);
  return items[0]!;
}

const article = "Drucker druckt nicht: Druckwarteschlange leeren";
const incident = "Drucker im 3. OG druckt nicht";

// configure is what an administrator sets up before the walk-through.
async function configure() {
  const api = await admin();
  const persons: Record<string, string> = {};
  for (const who of [nora, bernd, gesa]) {
    const p = await send(api, "POST", "/api/v1/records/core.person", {
      first_name: who.given_name,
      last_name: who.family_name,
      email: who.email,
    });
    persons[who.subject] = p["id"] as string;
  }

  const roles: { name: string; label: string; permissions: [string, string, string][] }[] = [
    {
      name: "sd21_portal",
      label: "Mitarbeitende (Portal)",
      permissions: [
        ["incident.incident", "create", "all"],
        ["incident.incident", "read", "involved"],
        ["incident.incident", "update", "involved"],
        ["request.request", "create", "all"],
        ["request.request", "read", "involved"],
        ["request.request", "update", "involved"],
      ],
    },
    {
      name: "sd21_servicedesk",
      label: "Servicedesk",
      permissions: [
        ["core.ui", "workplace", "all"],
        ["incident.incident", "read", "all"],
        ["incident.incident", "create", "all"],
        ["incident.incident", "update", "all"],
        ["request.request", "read", "all"],
        ["request.request", "update", "all"],
        ["request.task", "read", "all"],
        ["request.task", "update", "all"],
        ["knowledge.article", "read", "all"],
        ["knowledge.article", "update", "all"],
        ["core.person", "read", "all"],
        ["core.group", "read", "all"],
      ],
    },
  ];
  for (const role of roles) {
    await send(api, "POST", "/api/v1/roles", { name: role.name, label: { de: role.label } });
    for (const [type_name, action, scope] of role.permissions) {
      await send(api, "PUT", `/api/v1/roles/${role.name}/permissions`, {
        type_name,
        action,
        scope,
      });
    }
  }
  await send(api, "POST", "/api/v1/auth/rules", {
    name: "portal",
    condition: '"itsm-portal" in claims.groups || "itsm-servicedesk" in claims.groups',
    role: "sd21_portal",
  });
  await send(api, "POST", "/api/v1/auth/rules", {
    name: "servicedesk",
    condition: '"itsm-servicedesk" in claims.groups',
    role: "sd21_servicedesk",
  });

  // A new workplace needs the approval of Gesa.
  const approval = await send(api, "POST", "/api/v1/records/core.approval_template", {
    name: "Arbeitsplatz genehmigen",
    message: "Bitte genehmigen Sie den neuen Arbeitsplatz.",
    active: true,
  });
  await send(api, "POST", "/api/v1/records/core.approval_template_stage", {
    template: approval["id"],
    name: "Vorgesetzte",
    actors: { approvers: [{ type: "core.person", id: persons[gesa.subject] }] },
  });
  const workplace = await one(api, "request.template", "filter[name]=Neuer%20Arbeitsplatz");
  await send(
    api,
    "PATCH",
    `/api/v1/records/request.template/${workplace["id"] as string}`,
    { approval_template: approval["id"] },
    { "If-Match": `"${String(workplace["version"])}"` },
  );

  // An article for everyone, which Bernd as its editor is asked to approve.
  const art = await send(api, "POST", "/api/v1/records/knowledge.article", {
    title: article,
    body: "<p>Öffnen Sie die Druckwarteschlange, löschen Sie alle Aufträge und drucken Sie neu.</p>",
    audience: "all",
    actors: { editors: [{ type: "core.person", id: persons[bernd.subject] }] },
  });
  await send(api, "POST", `/api/v1/records/knowledge.article/${art["id"] as string}:transition`, {
    transition: "submit",
  });
  await api.dispose();
}

test("the walk-through of milestone 1", async ({ browser }) => {
  test.setTimeout(240_000);
  await configure();

  // 1. Bernd signs in through the provider and approves the article.
  const desk = await signIn(browser, bernd);
  await desk.goto("/approvals");
  await noFindings(desk);
  await desk.getByRole("link", { name: article }).click();
  await desk.getByRole("tab", { name: "Genehmigung" }).click();
  await desk.getByRole("button", { name: "Zustimmen" }).click();
  await expect(desk.getByRole("region", { name: "Status" })).toContainText("Veröffentlicht");

  // 2. Nora reports an incident in the portal; the article is suggested next to the form.
  const portal = await signIn(browser, nora);
  await expect(portal).toHaveURL(/\/portal$/);
  await portal.getByRole("link", { name: /Störung melden/ }).click();
  await expect(portal.getByRole("heading", { name: "Störung anlegen" })).toBeVisible();
  await portal.getByLabel(/^Titel/).fill(incident);
  await expect(portal.getByText(article).first()).toBeVisible();
  await noFindings(portal);
  await portal.getByRole("button", { name: "Anlegen", exact: true }).click();
  await expect(portal.getByRole("heading", { name: incident })).toBeVisible();
  await expect(portal).toHaveURL(/\/portal\/incident\.incident\//);
  const noraIncident = portal.url();

  // 3. Bernd finds it in the list of incidents, rates it, sees its deadline and solves it with
  // the article.
  await desk.goto("/types/incident.incident");
  // In the list Enter shows the preview, Umschalt Enter opens the page (UI53).
  await desk.getByRole("row", { name: new RegExp(incident) }).focus();
  await desk.keyboard.press("Shift+Enter");
  await expect(desk.getByRole("heading", { name: incident })).toBeVisible();
  await pick(desk.getByRole("combobox", { name: "Auswirkung" }), "high");
  await pick(desk.getByRole("combobox", { name: "Dringlichkeit" }), "high");
  await desk.getByRole("button", { name: "Speichern", exact: true }).click();
  await expect(desk.getByText("Version 2")).toBeVisible();
  // P1 from impact and urgency: the reaction is due in 15 minutes, round the clock.
  await expect(desk.getByText("Störung P1 Reaktion")).toBeVisible();
  await expect(desk.getByText(/Noch 1[45] min · fällig/).first()).toBeVisible();
  await noFindings(desk);
  await desk.getByRole("button", { name: "Beginnen", exact: true }).click();
  await expect(desk.getByRole("region", { name: "Status" })).toContainText("In Arbeit");
  await desk.getByRole("tab", { name: "Wissen" }).click();
  await desk.getByRole("button", { name: "Als Lösung übernehmen" }).first().click();
  await expect(desk.getByText(/im Formular noch nicht gespeichert/)).toBeVisible();
  await desk.getByRole("button", { name: "Speichern", exact: true }).click();
  await expect(desk.getByText("Version 4")).toBeVisible();
  await desk.getByRole("button", { name: "Lösen", exact: true }).click();
  await expect(desk.getByRole("region", { name: "Status" })).toContainText("Gelöst");

  // 4. Nora sees the solution in the portal and confirms it; the incident is closed.
  await portal.goto(noraIncident);
  await expect(portal.getByText("Öffnen Sie die Druckwarteschlange").first()).toBeVisible();
  await noFindings(portal);
  await portal.getByRole("button", { name: "Lösung bestätigen" }).click();
  await expect(portal.getByText("Geschlossen").first()).toBeVisible();
  await noFindings(portal);

  // 5. Nora orders a new workplace from the catalogue; the request waits for Gesa.
  await portal.goto("/portal");
  await portal
    .getByRole("main")
    .getByRole("link", { name: /^Katalog/ })
    .click();
  await portal.getByRole("link", { name: "Neuer Arbeitsplatz" }).click();
  await expect(portal.getByRole("heading", { name: "Neuer Arbeitsplatz" })).toBeVisible();
  await portal.getByRole("textbox", { name: /^Name/ }).fill("Paul Neu");
  await portal.getByLabel(/^Ab wann/).fill("2026-12-01");
  await pick(portal.getByRole("combobox", { name: "Gerät" }), "notebook");
  await noFindings(portal);
  await portal.getByRole("button", { name: "Bestellen", exact: true }).click();
  await expect(portal.getByText("Status: Neu")).toBeVisible();
  // The rule asks for the approval in the background; the portal page shows it after a reload.
  await expect(async () => {
    await portal.reload();
    await expect(portal.getByText("Status: Wartet auf Genehmigung")).toBeVisible({ timeout: 1000 });
  }).toPass({ timeout: 30_000 });
  await noFindings(portal);

  // 6. Gesa approves it in the portal; the request goes into fulfilment.
  const boss = await signIn(browser, gesa);
  await boss.getByRole("link", { name: "Genehmigungen" }).first().click();
  await boss
    .getByRole("link", { name: /Neuer Arbeitsplatz/ })
    .first()
    .click();
  await noFindings(boss);
  await boss.getByRole("button", { name: "Zustimmen" }).click();
  await expect(boss.getByText(/zugestimmt/i).first()).toBeVisible();

  // Bernd finds the request in fulfilment with the tasks of the template.
  await desk.goto("/types/request.request");
  await desk
    .getByRole("row", { name: /Neuer Arbeitsplatz/ })
    .first()
    .focus();
  await desk.keyboard.press("Shift+Enter");
  await expect(desk.getByRole("region", { name: "Status" })).toContainText("In Erfüllung");
  await expect(desk.getByText("Gerät bereitstellen").first()).toBeVisible();
  await expect(desk.getByText("Zugänge einrichten").first()).toBeVisible();
  await noFindings(desk);
});
