// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Filters and saved searches (UI4): conditions become the where of the JSON query and back;
// a filter narrows the list; saving keeps condition, columns and sort and puts the search in
// the address; a link opens it again; a search shared with one stands apart and cannot be
// changed from the list.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, probeType, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

import { fromWhere, toWhere } from "./Filters";

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

describe("TypePage with filters", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(9);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("turns conditions into a where and back", () => {
    expect(toWhere(probeType, [])).toBeUndefined();
    expect(toWhere(probeType, [{ field: "kind", op: "eq", value: "fault" }])).toEqual({
      field: "kind",
      eq: "fault",
    });
    const where = toWhere(probeType, [
      { field: "kind", op: "eq", value: "fault" },
      { field: "title", op: "contains", value: "" },
      { field: "due_on", op: "null", value: true },
    ]);
    expect(where).toEqual({
      and: [
        { field: "kind", eq: "fault" },
        { field: "due_on", null: true },
      ],
    });
    expect(fromWhere(where)).toEqual([
      { field: "kind", op: "eq", value: "fault" },
      { field: "due_on", op: "null", value: true },
    ]);
    // A moment takes the start or the end of the day.
    const day = toWhere(probeType, [{ field: "updated_at", op: "lte", value: "2026-10-29" }]) as {
      lte: string;
    };
    expect(new Date(day.lte).getHours()).toBe(23);
    // More than AND of conditions is shown as it is.
    expect(fromWhere({ or: [{ field: "kind", eq: "fault" }] })).toBeNull();
  });

  it("filters, saves the search, opens it by link, and keeps shared ones apart", async () => {
    const user = userEvent.setup();
    state.searches = [
      {
        id: "01a15a3c-8545-7691-b780-500000000099",
        version: 1,
        name: "Von Kollegin",
        target_type: "probe.probe",
        query: JSON.stringify({ field: "kind", eq: "wish" }),
        columns: "",
        sort: "",
        created_by: "01a15a3c-8545-7691-b780-0000000000ff",
      },
    ];
    const first = renderAt("/types/probe.probe");
    await screen.findByText("9 Datensätze");

    // Kind is fault: three of nine (kind cycles question, fault, wish).
    await user.click(screen.getByRole("button", { name: "Bedingung hinzufügen" }));
    const condition = screen.getByRole("listitem", { name: "Bedingung 1" });
    await user.selectOptions(within(condition).getByRole("combobox", { name: "Feld" }), "kind");
    await user.selectOptions(within(condition).getByRole("combobox", { name: "Wert" }), "fault");
    expect(await screen.findByText("3 Datensätze")).toBeInTheDocument();
    const query = state.calls.filter((c) => c.path === "/api/v1/records/probe.probe:query").at(-1)!;
    expect(JSON.parse(query.body!)).toMatchObject({ where: { field: "kind", eq: "fault" } });
    await expectAccessible(first.container);

    // Save it: the search keeps condition, columns and sort, and comes into the address.
    await user.click(screen.getByRole("button", { name: "Als neue Suche speichern" }));
    await user.type(screen.getByLabelText("Name der Suche"), "Störungen");
    await user.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() =>
      expect(first.router.state.location.search).toMatchObject({ search: expect.any(String) }),
    );
    const mine = state.searches.find((x) => x["name"] === "Störungen")!;
    expect(JSON.parse(String(mine["query"]))).toEqual({ field: "kind", eq: "fault" });
    expect(String(mine["columns"])).toContain("title");
    expect(mine["sort"]).toBe("-updated_at");
    expect(await screen.findByRole("heading", { name: /Probe.*Störungen/ })).toBeInTheDocument();
    const picker = screen.getByLabelText("Gespeicherte Suchen");
    expect(within(picker).getByRole("group", { name: "Eigene" })).toHaveTextContent("Störungen");
    expect(within(picker).getByRole("group", { name: "Mit mir geteilt" })).toHaveTextContent(
      "Von Kollegin",
    );
    first.unmount();

    // The link opens it again with its condition in the filters.
    renderAt(`/types/probe.probe?search=${String(mine["id"])}`);
    expect(await screen.findByText("3 Datensätze")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Wert" })).toHaveValue("fault");
    expect(screen.getByRole("button", { name: "Änderungen speichern" })).toBeInTheDocument();

    // A shared one: its own condition, no saving of changes, a page about it instead.
    await user.selectOptions(screen.getByLabelText("Gespeicherte Suchen"), "Von Kollegin");
    expect(await screen.findByRole("heading", { name: /Probe.*Von Kollegin/ })).toBeInTheDocument();
    expect(await screen.findByText("3 Datensätze")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Wert" })).toHaveValue("wish");
    expect(screen.queryByRole("button", { name: "Änderungen speichern" })).toBeNull();
    expect(screen.getByRole("link", { name: "Über diese Suche" })).toBeInTheDocument();
  });
});
