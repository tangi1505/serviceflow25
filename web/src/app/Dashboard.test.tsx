// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Dashboards (UI13): the widgets with the filters period and unit, a point that opens the list,
// editing with saving at once (refused as a whole when a widget is wrong), the start page.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { pack, range } from "@/app/Dashboard";
import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

const D = "01a15a3c-8545-7691-b780-d00000000001";
const W = (n: number) => `01a15a3c-8545-7691-b780-e0000000000${n}`;

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

function board(state: FakeState) {
  state.dashboards = [
    {
      dashboard: { id: D, name: "Service Desk Überblick", version: 1 },
      editable: true,
      widgets: [
        {
          id: W(1),
          title: "Offene",
          kind: "number",
          target_type: "probe.probe",
          x: 0,
          y: 0,
          w: 1,
          h: 1,
        },
        {
          id: W(2),
          title: "Nach Art",
          kind: "pie",
          target_type: "probe.probe",
          group_by: "kind",
          x: 1,
          y: 0,
          w: 2,
          h: 1,
        },
        {
          id: W(3),
          title: "Neueste",
          kind: "list",
          target_type: "probe.probe",
          x: 0,
          y: 1,
          w: 2,
          h: 1,
        },
        {
          id: W(4),
          title: "Hinweise",
          kind: "text",
          body: "<p>Wartung am Samstag</p>",
          x: 2,
          y: 1,
          w: 2,
          h: 1,
        },
      ],
    },
  ];
  const where = { field: "state", neq: "done" };
  state.widgetData = {
    [W(1)]: { kind: "number", type: "probe.probe", where, value: 42 },
    [W(2)]: {
      kind: "pie",
      type: "probe.probe",
      where,
      group: "kind",
      points: [
        { key: "fault", label: { de: "Störung" }, value: 2 },
        { key: "question", label: { de: "Frage" }, value: 1 },
      ],
    },
    [W(3)]: {
      kind: "list",
      type: "probe.probe",
      columns: ["title", "kind"],
      items: [
        { id: "01a15a3c-8545-7691-b780-f00000000001", version: 1, title: "Drucker", kind: "fault" },
      ],
    },
  };
}

describe("Dashboards", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(2);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("packs the widgets and turns the period into a range", () => {
    expect(
      pack([
        { w: 1, h: 1 },
        { w: 2, h: 1 },
        { w: 2, h: 1 },
        { w: 4, h: 1 },
      ]),
    ).toEqual([
      { x: 0, y: 0 },
      { x: 1, y: 0 },
      { x: 0, y: 1 },
      { x: 0, y: 2 },
    ]);
    const now = new Date(2026, 10, 13, 15, 30);
    expect(range("all", { from: "", to: "" }, now)).toEqual({});
    expect(range("today", { from: "", to: "" }, now).from).toBe(
      new Date(2026, 10, 13).toISOString(),
    );
    expect(range("own", { from: "2026-11-01", to: "2026-11-02" }, now)).toEqual({
      from: new Date(2026, 10, 1).toISOString(),
      to: new Date(2026, 10, 3).toISOString(),
    });
  });

  it("shows the widgets with the filters, and a point opens the list behind it", async () => {
    board(state);
    const user = userEvent.setup();
    const { container, router } = renderAt(`/dashboards/${D}`);
    await screen.findByRole("heading", { name: "Service Desk Überblick" });
    const number = await screen.findByRole("button", { name: "Offene: 42, Liste öffnen" });
    const pie = screen.getByRole("region", { name: "Nach Art" });
    expect(await within(pie).findByRole("button", { name: /^Störung · 2/ })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "Drucker" })).toBeInTheDocument();
    expect(within(pie).getByText("(67 %)", { exact: false })).toBeInTheDocument();
    expect(screen.getByText("Wartung am Samstag")).toBeInTheDocument();
    // The navigation lists the dashboard.
    expect(screen.getByRole("link", { name: "Service Desk Überblick" })).toBeInTheDocument();
    await expectAccessible(container);

    // The filters go to every widget.
    await user.click(screen.getByRole("radio", { name: "7 Tage" }));
    await waitFor(() => {
      const last = state.calls
        .filter((c) => c.path === `/api/v1/dashboards/${D}/widgets/${W(1)}/data`)
        .at(-1);
      expect(JSON.parse(String(last!.body)) as Record<string, unknown>).toHaveProperty("from");
    });
    await user.selectOptions(screen.getByLabelText("Einheit"), "root.it");
    await waitFor(() => {
      const last = state.calls
        .filter((c) => c.path === `/api/v1/dashboards/${D}/widgets/${W(2)}/data`)
        .at(-1);
      expect((JSON.parse(String(last!.body)) as Record<string, unknown>)["unit"]).toBe("root.it");
    });

    // A point opens the list with the condition and the group.
    await user.click(within(pie).getByRole("button", { name: /Störung · 2/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/types/probe.probe"));
    expect(JSON.parse(String((router.state.location.search as { where?: string }).where))).toEqual({
      and: [
        { field: "state", neq: "done" },
        { field: "kind", eq: "fault" },
      ],
    });
    void number;
  });

  it("edits: adds a widget, refuses a wrong one as a whole, and saves all at once", async () => {
    board(state);
    const user = userEvent.setup();
    const { container } = renderAt(`/dashboards/${D}`);
    await user.click(await screen.findByRole("button", { name: "Bearbeiten" }));
    const bar = screen.getByRole("region", { name: "Speichern" });
    expect(within(bar).getByRole("status")).toHaveTextContent("Alles gespeichert");
    expect(within(bar).getByRole("button", { name: "Speichern" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Offene verschieben" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Widget hinzufügen" }));
    const form = screen.getByRole("region", { name: "Widget „Neues Widget“" });
    await user.clear(within(form).getByLabelText("Titel"));
    await user.type(within(form).getByLabelText("Titel"), "Je Art");
    await user.selectOptions(within(form).getByLabelText("Art"), "bar");
    await user.selectOptions(within(form).getByLabelText("Typ"), "probe.probe");
    await user.selectOptions(within(form).getByLabelText("Gruppieren nach"), "kind");
    await user.selectOptions(within(form).getByLabelText("Breite"), "2");
    expect(within(bar).getByRole("status")).toHaveTextContent("Änderungen nicht gespeichert");
    await expectAccessible(container);

    // The server refuses one widget: nothing is saved, the message stands at it.
    state.dashboardInvalid = [
      { field: "widgets[4].group_by", code: "invalid", message: "only a date groups by hour" },
    ];
    await user.click(within(bar).getByRole("button", { name: "Speichern" }));
    expect(
      await screen.findByText("Nicht gespeichert: Bitte die markierten Widgets prüfen."),
    ).toBeInTheDocument();
    expect(screen.getByText("only a date groups by hour")).toBeInTheDocument();
    expect(state.dashboards![0]!.widgets).toHaveLength(4);

    state.dashboardInvalid = undefined;
    await user.click(within(bar).getByRole("button", { name: "Speichern" }));
    await screen.findByRole("button", { name: "Bearbeiten" });
    const saved = state.dashboards![0]!.widgets;
    expect(saved).toHaveLength(5);
    expect(saved[4]).toMatchObject({
      title: "Je Art",
      kind: "bar",
      target_type: "probe.probe",
      group_by: "kind",
      w: 2,
      x: 0,
      y: 2,
    });
    const put = state.calls.find((c) => c.method === "PUT" && c.path === `/api/v1/dashboards/${D}`);
    expect(JSON.parse(String(put!.body)) as Record<string, unknown>).toMatchObject({ version: 1 });
  });

  it("shows the chosen dashboard on the start page, chosen in my settings", async () => {
    board(state);
    state.settings = { start_dashboard: D };
    const user = userEvent.setup();
    const first = renderAt("/");
    await screen.findByRole("heading", { name: "Willkommen, Pascal" });
    expect(
      await screen.findByRole("heading", { name: "Service Desk Überblick", level: 2 }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("button", { name: "Offene: 42, Liste öffnen" }),
    ).toBeInTheDocument();
    first.unmount();

    renderAt("/settings");
    const start = await screen.findByLabelText("Startseite");
    expect(
      await within(start).findByRole("option", { name: "Service Desk Überblick" }),
    ).toBeInTheDocument();
    await user.selectOptions(start, "none");
    await user.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(state.settings).toMatchObject({ start_dashboard: "none" }));
  });

  // UI13b: a widget of a dashboard the session reads stands in the sidebar of every record of
  // its type, computed for that record; in the editor its condition is written as JSON.
  it("shows a widget in the sidebar of a record, and sets one up in the editor", async () => {
    board(state);
    state.sidebarWidgets = [
      { id: W(5), title: "Gleiche Art", kind: "number", sidebar_type: "probe.probe" },
    ];
    state.widgetData = {
      ...state.widgetData,
      [W(5)]: { kind: "number", type: "probe.probe", value: 7 },
    };
    const user = userEvent.setup();
    const rid = String(state.records[0]!["id"]);
    const first = renderAt(`/types/probe.probe/${rid}`);
    const widget = await screen.findByRole("region", { name: "Gleiche Art" });
    expect(
      await within(widget).findByRole("button", { name: "Gleiche Art: 7, Liste öffnen" }),
    ).toBeInTheDocument();
    expect(
      state.calls.some(
        (c) =>
          c.method === "POST" &&
          c.path === `/api/v1/records/probe.probe/${rid}/widgets/${W(5)}/data`,
      ),
    ).toBe(true);
    first.unmount();

    renderAt(`/dashboards/${D}`);
    await user.click(await screen.findByRole("button", { name: "Bearbeiten" }));
    const form = screen.getByRole("region", { name: "Widget „Offene“" });
    await user.selectOptions(
      within(form).getByLabelText("Auch in der Seitenleiste von"),
      "probe.probe",
    );
    const json = within(form).getByLabelText("Bedingung (JSON)");
    await user.click(json);
    await user.paste('{"field": "kind", "eq": "@record.kind"}');
    await user.click(screen.getByRole("button", { name: "Speichern" }));
    await screen.findByRole("button", { name: "Bearbeiten" });
    expect(state.dashboards![0]!.widgets[0]).toMatchObject({
      sidebar_type: "probe.probe",
      query: '{"field": "kind", "eq": "@record.kind"}',
    });
  });
});
