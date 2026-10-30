// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Mass actions (UI6): choose all loaded rows, set a field in one call with the versions the
// list loaded, see the result per row with the conflict of a record changed meanwhile; run a
// transition that applies to some only; the trash asks once more.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";
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

describe("Batch", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(6);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("sets a field on all loaded rows and shows the conflict of one", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/types/probe.probe");
    await screen.findByText("6 Datensätze");
    await user.click(screen.getByRole("checkbox", { name: "Alle geladenen auswählen" }));
    const bar = await screen.findByRole("region", { name: "6 gewählt" });
    await user.selectOptions(within(bar).getByRole("combobox", { name: "Feld" }), "kind");
    await user.selectOptions(within(bar).getByRole("combobox", { name: "Neuer Wert" }), "fault");
    await expectAccessible(container);

    // Someone changes one record after the list loaded it.
    state.records[2]!["version"] = 5;
    await user.click(within(bar).getByRole("button", { name: "Auf 6 anwenden" }));
    expect(
      await within(bar).findByText("5 geändert · 1 inzwischen geändert (Konflikt)"),
    ).toBeInTheDocument();
    expect(within(bar).getByRole("listitem")).toHaveTextContent("Probe 000003");
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
  });

  it("runs a transition, and the trash only after asking again", async () => {
    const user = userEvent.setup();
    state.records[0]!["state"] = "in_progress";
    renderAt("/types/probe.probe");
    await screen.findByText("6 Datensätze");
    await user.click(screen.getByRole("checkbox", { name: "Auswählen: Probe 000001" }));
    await user.click(screen.getByRole("checkbox", { name: "Auswählen: Probe 000002" }));
    const bar = await screen.findByRole("region", { name: "2 gewählt" });
    await user.selectOptions(within(bar).getByRole("combobox", { name: "Aktion" }), "transition");
    await user.selectOptions(within(bar).getByRole("combobox", { name: "Übergang" }), "start");
    await user.click(within(bar).getByRole("button", { name: "Auf 2 anwenden" }));
    const summary = (await within(bar).findAllByText(/abgelehnt/))[0]!;
    expect(summary).toHaveTextContent("1 abgelehnt (Übergang gilt nicht)");
    expect(summary).toHaveTextContent("1 geändert");

    await user.selectOptions(within(bar).getByRole("combobox", { name: "Aktion" }), "trash");
    await user.click(within(bar).getByRole("button", { name: "Auf 2 anwenden" }));
    expect(state.calls.filter((c) => c.path === "/api/v1/records/probe.probe:batch")).toHaveLength(
      1,
    );
    await user.click(within(bar).getByRole("button", { name: "Wirklich 2 in den Papierkorb" }));
    await waitFor(() => expect(state.records).toHaveLength(4));
  });
});
