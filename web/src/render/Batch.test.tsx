// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Mass actions (UI6, UI61): choose all loaded rows, set a field in one call with the versions the
// list loaded; the row of the action stands in place of the column heads; the result comes as a
// message, yellow with the conflict of a record changed meanwhile; a transition that applies to
// some only; the trash without asking, with "Rückgängig" in its message.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";
import { toast, Toaster } from "@/ui/toast";

function renderAt(path: string) {
  const queryClient = createQueryClient();
  const router = createAppRouter({ queryClient, history: memoryHistory(path) });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      <Toaster />
    </QueryClientProvider>,
  );
  return { router, ...view };
}

async function choose(user: ReturnType<typeof userEvent.setup>, name: string, option: string) {
  await user.click(screen.getByRole("combobox", { name }));
  await user.click(await screen.findByRole("option", { name: option }));
}

describe("Batch", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(6);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    act(() => toast.clear());
    vi.unstubAllGlobals();
  });

  it("sets a field on all loaded rows and reports the conflict of one", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/types/probe.probe");
    await screen.findByText("6 Datensätze");
    const grid = screen.getByRole("grid");
    const firstRow = within(grid).getAllByRole("row")[1]!;
    const top = firstRow.style.transform;
    await user.click(screen.getByRole("checkbox", { name: "Alle geladenen auswählen" }));
    // The row of the action replaces the column heads; the count stands once.
    const bar = await within(grid).findByRole("group", { name: "6 ausgewählt" });
    expect(screen.getAllByText("6 ausgewählt")).toHaveLength(1);
    expect(within(bar).getByRole("button", { name: "Anwenden" })).toBeInTheDocument();
    expect(within(grid).getAllByRole("row")[1]!.style.transform).toBe(top);
    await choose(user, "Feld", "Art");
    await choose(user, "Neuer Wert", "Störung");
    await expectAccessible(container);

    // Someone changes one record after the list loaded it.
    state.records[2]!["version"] = 5;
    await user.click(within(bar).getByRole("button", { name: "Anwenden" }));
    const message = await screen.findByRole("alert");
    expect(message).toHaveTextContent("5 von 6 geändert");
    expect(message).toHaveTextContent("Probe 000003: inzwischen geändert");
    const call = state.calls.filter((c) => c.path === "/api/v1/records/probe.probe:batch").at(-1)!;
    const body = JSON.parse(call.body!) as { rows: { id: string; version: number; set: object }[] };
    expect(body.rows).toHaveLength(6);
    // In the order of the list (newest first), each with the version it loaded.
    expect(body.rows.find((r) => r.id === state.records[0]!["id"])).toEqual({
      id: state.records[0]!["id"],
      version: 1,
      set: { kind: "fault" },
    });
    expect(state.records.filter((r) => r["kind"] === "fault")).toHaveLength(5);

    await user.click(within(bar).getByRole("button", { name: "Auswahl aufheben" }));
    expect(within(grid).queryByRole("group")).not.toBeInTheDocument();
  });

  it("runs a transition, and the trash at once with Rückgängig", async () => {
    const user = userEvent.setup();
    state.records[0]!["state"] = "in_progress";
    renderAt("/types/probe.probe");
    await screen.findByText("6 Datensätze");
    await user.click(screen.getByRole("checkbox", { name: "Auswählen: Probe 000001" }));
    await user.click(screen.getByRole("checkbox", { name: "Auswählen: Probe 000002" }));
    const bar = await screen.findByRole("group", { name: "2 ausgewählt" });
    await choose(user, "Aktion", "Übergang ausführen");
    await choose(user, "Übergang", "Beginnen");
    await user.click(within(bar).getByRole("button", { name: "Anwenden" }));
    const partly = await screen.findByRole("alert");
    expect(partly).toHaveTextContent("1 von 2 geändert");
    expect(partly).toHaveTextContent("Übergang gilt hier nicht");
    act(() => toast.clear());

    await choose(user, "Aktion", "In den Papierkorb");
    await user.click(within(bar).getByRole("button", { name: "Anwenden" }));
    await waitFor(() => expect(state.records).toHaveLength(4));
    const done = (await screen.findByText("2 in den Papierkorb gelegt")).closest<HTMLElement>(
      '[role="status"]',
    )!;
    await user.click(within(done).getByRole("button", { name: "Rückgängig" }));
    await waitFor(() => expect(state.records).toHaveLength(6));
    expect(await screen.findByText("2 aus dem Papierkorb zurückgeholt")).toBeInTheDocument();
  });
});
