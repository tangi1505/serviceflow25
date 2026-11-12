// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Approvals (WF6, WF-Plan E17): the tab "Genehmigung" shows the stages with who decided, also
// "für" a person; whoever is asked agrees, or refuses only with a reason; asking builds stages.
// "Zu genehmigen" lists the open questions in the workspace and as the area of the portal, and
// the portal shows the record to answer it. The substitutes stand under "Meine Einstellungen".
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Approval, TypeDefinition } from "@/api/client";
import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { approverPerson, fakeApi, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

function renderAt(path: string) {
  const queryClient = createQueryClient();
  const router = createAppRouter({ queryClient, history: memoryHistory(path) });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return { router, ...view };
}

const orderType: TypeDefinition = {
  name: "order",
  package: "probe",
  label: { de: "Bestellung" },
  title_field: "title",
  traits: ["history", "approvals"],
  fields: [
    { name: "title", type: "text", label: { de: "Titel" }, searchable: true },
    { name: "reason", type: "text", label: { de: "Begründung" } },
  ],
  states: [
    { name: "draft", label: { de: "Neu" }, initial: true },
    { name: "waiting", label: { de: "Wartet auf Genehmigung" } },
  ],
};
const personType: TypeDefinition = {
  name: "person",
  package: "core",
  label: { de: "Person" },
  title_field: "last_name",
  fields: [
    { name: "first_name", type: "text", searchable: true },
    { name: "last_name", type: "text", searchable: true },
  ],
};
const order = "01a15a3c-8545-7691-b780-d00000000001";
const lead = "01a15a3c-8545-7691-b780-a00000000001";
const sub = "01a15a3c-8545-7691-b780-a00000000002";
const sec = "01a15a3c-8545-7691-b780-a00000000003";
const group = "01a15a3c-8545-7691-b780-b00000000001";
const groupStep = "0190a000-0000-7000-9000-000000000102";

function running(): Approval {
  return {
    id: "0190a000-0000-7000-8000-000000000001",
    state: "open",
    stage: 2,
    message: "Bitte bis Freitag",
    requester: approverPerson,
    requested_at: "2026-11-10T07:40:00Z",
    stages: [
      {
        position: 1,
        name: "Fachliche Prüfung",
        need: 0,
        state: "approved",
        steps: [
          {
            id: "0190a000-0000-7000-9000-000000000001",
            approver: { type: "core.person", id: lead },
            state: "approved",
            comment: "In Ordnung",
            decided_by: sub,
            on_behalf: lead,
            decided_at: "2026-11-10T08:12:00Z",
            mine: false,
          },
        ],
      },
      {
        position: 2,
        name: "Informationssicherheit",
        need: 2,
        state: "open",
        steps: [
          {
            id: "0190a000-0000-7000-9000-000000000101",
            approver: { type: "core.person", id: sec },
            state: "approved",
            decided_by: sec,
            decided_at: "2026-11-10T09:03:00Z",
            mine: false,
          },
          { id: groupStep, approver: { type: "core.group", id: group }, state: "open", mine: true },
        ],
      },
    ],
  };
}

describe("Approvals", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.extraTypes = [orderType, personType];
    state.extraRecords = {
      "probe.order": [
        {
          id: order,
          version: 1,
          state: "waiting",
          title: "Zugang Gruppenlaufwerk Nord",
          approval: "pending",
        },
      ],
    };
    state.persons = {
      [lead]: { id: lead, first_name: "Anna", last_name: "Leitung" },
      [sub]: { id: sub, first_name: "Ben", last_name: "Vertreter" },
      [sec]: { id: sec, first_name: "Karl", last_name: "Sicher" },
    };
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the stages and answers the open question, a refusal only with a reason", async () => {
    state.approvals = { [order]: [running()] };
    const user = userEvent.setup();
    const { container } = renderAt(`/types/probe.order/${order}`);
    const tab = await screen.findByRole("tab", { name: "Genehmigung" });
    expect(
      screen
        .getAllByRole("tab")
        .map((t) => t.textContent)
        .slice(0, 2),
    ).toEqual(["Bezüge", "Genehmigung"]);
    await user.click(tab);
    const panel = screen.getByRole("tabpanel");
    expect(
      await within(panel).findByRole("heading", { name: "Stufe 1 von 2 · Fachliche Prüfung" }),
    ).toBeInTheDocument();
    expect(panel).toHaveTextContent("Stufe 2 von 2 · Informationssicherheit");
    expect(panel).toHaveTextContent("mindestens 2 von 2 · 1 Zustimmung");
    expect(panel).toHaveTextContent("Bitte bis Freitag");
    // Decided by the substitute for the lead.
    expect(await within(panel).findByText("Ben Vertreter")).toBeInTheDocument();
    expect(panel).toHaveTextContent("Ben Vertreter für Anna Leitung");
    expect(panel).toHaveTextContent("In Ordnung");
    expect(within(panel).getByRole("button", { name: "Zurückziehen" })).toBeInTheDocument();
    await expectAccessible(container);

    // A refusal without a reason stays here.
    await user.click(within(panel).getByRole("button", { name: "Ablehnen" }));
    expect(within(panel).getByRole("alert")).toHaveTextContent("Bitte nennen Sie einen Grund");
    expect(state.calls.some((c) => c.path.endsWith(":decide"))).toBe(false);

    await user.type(within(panel).getByLabelText(/Kommentar/), "Passt");
    await user.click(within(panel).getByRole("button", { name: "Zustimmen" }));
    await waitFor(() => expect(state.calls.some((c) => c.path.endsWith(":decide"))).toBe(true));
    const sent = state.calls.find((c) => c.path.endsWith(":decide"))!;
    expect(sent.path).toBe(`/api/v1/records/probe.order/${order}/approvals/${running().id}:decide`);
    expect(JSON.parse(sent.body!)).toEqual({
      decision: "approve",
      comment: "Passt",
      step: groupStep,
    });
  });

  // SD17 (SD-Plan N22): a change that waits shows as a band, its fields are locked, the tab
  // shows it word by word, and the band withdraws it.
  it("shows a change that waits for approval, locks its fields and withdraws it", async () => {
    state.extraTypes = [
      { ...orderType, approval: { hold: { fields: ["title"], approvers: "editors" } } },
      personType,
    ];
    state.extraRecords!["probe.order"]![0]!["title"] = "VPN verbindet nicht";
    state.approvals = {
      [order]: [
        {
          id: "0190a000-0000-7000-8000-000000000017",
          state: "open",
          stage: 1,
          message: "Bitte geben Sie diese Änderung frei.",
          requester: approverPerson,
          requested_at: "2026-11-11T08:00:00Z",
          changes: { title: "VPN verbindet nicht oder bricht ab" },
          stages: [
            {
              position: 1,
              name: "",
              need: 1,
              state: "open",
              steps: [
                {
                  id: "0190a000-0000-7000-9000-000000000171",
                  approver: { type: "core.group", id: group },
                  state: "open",
                  mine: true,
                },
              ],
            },
          ],
        },
      ],
    };
    const user = userEvent.setup();
    const { container } = renderAt(`/types/probe.order/${order}`);
    const band = await screen.findByText(/Eine Änderung an Titel wartet auf Freigabe/);
    expect(screen.getByLabelText(/^Titel/)).toHaveAttribute("readonly");
    expect(screen.getByLabelText(/^Begründung/)).not.toHaveAttribute("readonly");
    await expectAccessible(container);

    await user.click(screen.getByRole("button", { name: "Ansehen" }));
    expect(screen.getByRole("tab", { name: "Genehmigung" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    const change = await within(screen.getByRole("tabpanel")).findByRole("region", {
      name: "Beantragte Änderung",
    });
    expect(
      within(change).getByText("oder bricht ab", { exact: false, selector: "ins" }),
    ).toBeVisible();

    await user.click(
      within(band.closest("[role=status]") as HTMLElement).getByRole("button", {
        name: "Änderung zurückziehen",
      }),
    );
    await waitFor(() => expect(state.calls.some((c) => c.path.endsWith(":withdraw"))).toBe(true));
  });

  it("asks for approval in stages", async () => {
    const user = userEvent.setup();
    renderAt(`/types/probe.order/${order}`);
    await user.click(await screen.findByRole("tab", { name: "Genehmigung" }));
    const panel = screen.getByRole("tabpanel");
    expect(
      await within(panel).findByText("Noch keine Genehmigung angefordert."),
    ).toBeInTheDocument();
    await user.click(within(panel).getByRole("button", { name: "Genehmigung anfordern" }));
    const send = within(panel).getByRole("button", { name: "Anfordern" });
    expect(send).toBeDisabled();
    await user.type(within(panel).getByLabelText("Name der Stufe"), "Fachliche Prüfung");
    await user.type(within(panel).getByRole("combobox", { name: /Genehmiger/ }), "leit");
    await user.click(await screen.findByRole("option", { name: /Leitung/ }));
    await user.click(within(panel).getByRole("button", { name: "Stufe hinzufügen" }));
    expect(
      within(panel)
        .getAllByRole("group")
        .filter((g) => g.tagName === "FIELDSET"),
    ).toHaveLength(2);
    expect(send).toBeDisabled();
    await user.click(within(panel).getAllByRole("button", { name: "Stufe entfernen" })[1]!);
    await user.type(within(panel).getByLabelText("Text an die Genehmiger"), "Bitte bis Freitag");
    await user.click(send);
    await waitFor(() =>
      expect(state.calls.some((c) => c.method === "POST" && c.path.endsWith("/approvals"))).toBe(
        true,
      ),
    );
    const post = state.calls.find((c) => c.method === "POST" && c.path.endsWith("/approvals"))!;
    expect(JSON.parse(post.body!)).toEqual({
      message: "Bitte bis Freitag",
      stages: [
        { name: "Fachliche Prüfung", need: 0, approvers: [{ type: "core.person", id: lead }] },
      ],
    });
    expect(await within(panel).findByText("Stufe 1 von 1 · Fachliche Prüfung")).toBeInTheDocument();
  });

  // WF7 (WF-Plan E18): a template fills the text and the stages, a role shows where its
  // person comes from, the stages may still change; the request names the template. A stage
  // shows how often it reminded and when it reminds next.
  it("asks from a template and shows the reminders", async () => {
    const user = userEvent.setup();
    state.approvalTemplates = {
      [order]: [
        {
          template: "0190a000-0000-7000-7000-000000000001",
          name: "Software mit Lizenzkosten",
          message: "Bitte genehmigen Sie die Lizenzkosten.",
          remind_days: 1,
          stages: [
            {
              name: "Fachliche Freigabe",
              need: 1,
              approvers: [{ type: "core.person", id: lead, role: "group.responsible" }],
            },
            { name: "Lizenzmanagement", need: 0, approvers: [{ type: "core.person", id: sec }] },
          ],
        },
      ],
    };
    renderAt(`/types/probe.order/${order}`);
    await user.click(await screen.findByRole("tab", { name: "Genehmigung" }));
    const panel = screen.getByRole("tabpanel");
    await user.click(await within(panel).findByRole("button", { name: "Genehmigung anfordern" }));
    await user.selectOptions(
      await within(panel).findByLabelText("Vorlage"),
      "Software mit Lizenzkosten",
    );
    expect(within(panel).getByLabelText("Text an die Genehmiger")).toHaveValue(
      "Bitte genehmigen Sie die Lizenzkosten.",
    );
    expect(
      within(panel)
        .getAllByLabelText("Name der Stufe")
        .map((x) => (x as HTMLInputElement).value),
    ).toEqual(["Fachliche Freigabe", "Lizenzmanagement"]);
    expect(within(panel).getByText(/aus der Rolle Leitung von group/)).toBeInTheDocument();
    expect(
      within(panel).getByText(/Erinnerung jeden Arbeitstag \(aus der Vorlage\)/),
    ).toBeInTheDocument();
    await expectAccessible(panel);
    await user.click(within(panel).getByRole("button", { name: "Anfordern" }));
    await waitFor(() =>
      expect(state.calls.some((c) => c.method === "POST" && c.path.endsWith("/approvals"))).toBe(
        true,
      ),
    );
    const post = state.calls.find((c) => c.method === "POST" && c.path.endsWith("/approvals"))!;
    expect(JSON.parse(post.body!)).toMatchObject({
      template: "0190a000-0000-7000-7000-000000000001",
      message: "Bitte genehmigen Sie die Lizenzkosten.",
      stages: [
        { name: "Fachliche Freigabe", need: 1, approvers: [{ type: "core.person", id: lead }] },
        { name: "Lizenzmanagement", need: 0, approvers: [{ type: "core.person", id: sec }] },
      ],
    });
  });

  it("shows how often a stage reminded", async () => {
    const a = running();
    a.stages[1]!.reminders = 2;
    a.stages[1]!.reminded_at = "2026-11-17T09:05:00Z";
    a.stages[1]!.remind_at = "2026-11-19T09:05:00Z";
    state.approvals = { [order]: [a] };
    const user = userEvent.setup();
    renderAt(`/types/probe.order/${order}`);
    await user.click(await screen.findByRole("tab", { name: "Genehmigung" }));
    const panel = screen.getByRole("tabpanel");
    expect(
      await within(panel).findByText(/Erinnert: 2-mal, zuletzt .* · nächste Erinnerung/),
    ).toBeInTheDocument();
  });

  it("lists what is to approve, in the workspace and as the area of the portal", async () => {
    state.approvals = { [order]: [running()] };
    state.myApprovals = {
      open: [
        {
          approval: running().id,
          step: groupStep,
          type_name: "probe.order",
          record_id: order,
          title: "Zugang Gruppenlaufwerk Nord",
          stage: 2,
          stages: 2,
          stage_name: "Informationssicherheit",
          approver: { type: "core.group", id: group },
          state: "open",
          on_behalf: lead,
          requester: approverPerson,
          requested_at: "2026-11-10T07:40:00Z",
        },
      ],
    };
    const user = userEvent.setup();
    const first = renderAt("/approvals");
    const nav = await screen.findByRole("link", { name: /Zu genehmigen/ });
    await waitFor(() => expect(nav).toHaveTextContent("Zu genehmigen1"));
    const row = await screen.findByRole("link", { name: "Zugang Gruppenlaufwerk Nord" });
    expect(row).toHaveAttribute("href", `/types/probe.order/${order}`);
    expect(screen.getByRole("table")).toHaveTextContent("in Vertretung für Anna Leitung");
    expect(screen.getByRole("table")).toHaveTextContent("Stufe 2 von 2 · Informationssicherheit");
    await expectAccessible(first.container);
    await user.click(screen.getByRole("button", { name: "Erledigt" }));
    expect(await screen.findByText("Noch nichts entschieden.")).toBeInTheDocument();
    first.unmount();

    // The portal: the area in the head with the number, the record to read and answer.
    const { container } = renderAt("/portal/approvals");
    const areas = await screen.findByRole("navigation", { name: "Bereiche" });
    const area = within(areas).getByRole("link", { name: /Genehmigungen/ });
    await waitFor(() => expect(within(area).getByLabelText("1 offen")).toBeInTheDocument());
    await user.click(await screen.findByRole("link", { name: "Zugang Gruppenlaufwerk Nord" }));
    expect(
      await screen.findByRole("heading", { name: "Zugang Gruppenlaufwerk Nord", level: 1 }),
    ).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Zustimmen" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Zurückziehen" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Genehmigung anfordern" })).toBeNull();
    await expectAccessible(container);
  });

  it("names and removes a substitute at the person", async () => {
    state.extraRecords = {
      ...state.extraRecords,
      // The persons as records of their own type: the page of the lead, the names of the others;
      // the fake search looks in title.
      "core.person": [
        { id: lead, version: 1, first_name: "Anna", last_name: "Leitung", title: "Leitung" },
        { id: sub, version: 1, first_name: "Ben", last_name: "Vertreter", title: "Vertreter" },
        { id: sec, version: 1, first_name: "Karl", last_name: "Sicher", title: "Sicher" },
      ],
    };
    state.substitutes = [
      {
        id: "0190b000-0000-7000-8000-000000000009",
        person: lead,
        substitute: sec,
        valid_from: "2026-11-01T00:00:00Z",
        valid_until: "2026-11-05T00:00:00Z",
        active: false,
      },
    ];
    const user = userEvent.setup();
    const { container } = renderAt(`/types/core.person/${lead}`);
    await user.click(await screen.findByRole("tab", { name: "Vertretungen" }));
    const panel = screen.getByRole("tabpanel");
    expect(await within(panel).findByText("Karl Sicher")).toBeInTheDocument();
    expect(panel).toHaveTextContent("nein");
    await user.type(within(panel).getByRole("combobox", { name: "Vertretung" }), "vertr");
    await user.click(await screen.findByRole("option", { name: /Vertreter/ }));
    await user.click(within(panel).getByRole("button", { name: "Hinzufügen" }));
    await waitFor(() =>
      expect(
        state.calls.some(
          (c) => c.method === "POST" && c.path === `/api/v1/persons/${lead}/substitutes`,
        ),
      ).toBe(true),
    );
    const post = state.calls.find((c) => c.method === "POST" && c.path.endsWith("/substitutes"))!;
    expect(JSON.parse(post.body!)).toEqual({ substitute: sub });
    expect(await within(panel).findByText("Ben Vertreter")).toBeInTheDocument();
    expect(panel).toHaveTextContent("unbefristet");
    await expectAccessible(container);
    await user.click(within(panel).getAllByRole("button", { name: "Entfernen" })[0]!);
    await waitFor(() => expect(state.substitutes).toHaveLength(1));
  });
});
