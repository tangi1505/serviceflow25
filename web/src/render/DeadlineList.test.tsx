// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The deadline column of the list (UI34, UI-Plan E37): shown where clocks run, red with text
// when breached, yellow when close, paused as a word; the filter finds the breached and the
// close ones against the moment of the query, and a saved search reads them back.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, probeType, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

import { deadlineOf } from "./DeadlineCell";
import { fromWhere, toWhere } from "./Filters";

function renderAt(path: string) {
  const queryClient = createQueryClient();
  const router = createAppRouter({ queryClient, history: memoryHistory(path) });
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

const minutes = (n: number) => new Date(Date.now() + n * 60_000).toISOString();

describe("Deadline column", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(5);
    state.signedIn = true;
    localStorage.clear();
    // 1 breached, 2 close (past three quarters of 60 minutes), 3 in time, 4 paused, 5 none.
    Object.assign(state.records[0]!, { sla_due_at: minutes(-15), sla_warn_at: minutes(-30) });
    Object.assign(state.records[1]!, { sla_due_at: minutes(10), sla_warn_at: minutes(-5) });
    Object.assign(state.records[2]!, { sla_due_at: minutes(50), sla_warn_at: minutes(35) });
    Object.assign(state.records[3]!, { sla_paused: true });
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => vi.unstubAllGlobals());

  it("shows the deadline where clocks run, with the state as text", async () => {
    state.timed = true;
    const view = renderAt("/types/probe.probe");
    expect(await screen.findByRole("columnheader", { name: /Frist/ })).toBeInTheDocument();
    const row = (n: number) => screen.getByRole("row", { name: new RegExp(`Probe 00000${n}`) });
    await screen.findByRole("row", { name: /Probe 000001/ });
    expect(within(row(1)).getByText(/Überfällig seit 15 min/)).toBeInTheDocument();
    expect(within(row(2)).getByText(/Noch (9|10) min/)).toBeInTheDocument();
    expect(within(row(3)).getByText(/Noch (49|50) min/)).toBeInTheDocument();
    expect(within(row(4)).getByText("Angehalten")).toBeInTheDocument();
    expect(within(row(5)).getByText("–")).toBeInTheDocument();
    // The list asks for the three-quarter point and the pause along with the deadline.
    const list = state.calls.filter((c) => c.path === "/api/v1/records/probe.probe").at(-1)!;
    expect(list.query.get("fields")).toContain("sla_warn_at");
    await expectAccessible(view.container);
  });

  it("has no deadline column where no clocks run", async () => {
    renderAt("/types/probe.probe");
    await screen.findByRole("row", { name: /Probe 000001/ });
    expect(screen.queryByRole("columnheader", { name: /Frist/ })).toBeNull();
  });

  it("filters the breached and the close ones against the moment of the query", async () => {
    state.timed = true;
    const user = userEvent.setup();
    renderAt("/types/probe.probe");
    await screen.findByRole("row", { name: /Probe 000001/ });
    // In the filter row (UI50): the deadline, arrow left to the comparison, "überschritten".
    await user.click(screen.getByRole("combobox", { name: "Filter setzen" }));
    await user.keyboard("frist{Enter}{ArrowLeft}über{Enter}");
    expect(screen.getByRole("button", { name: /Frist\s*ist überschritten/ })).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("row", { name: /Probe 000002/ })).toBeNull());
    expect(screen.getByRole("row", { name: /Probe 000001/ })).toBeInTheDocument();
    let query = state.calls.filter((c) => c.path === "/api/v1/records/probe.probe:query").at(-1)!;
    expect(JSON.parse(query.body!)).toMatchObject({ where: { field: "sla_due_at", lte: "@now" } });
    // A click on the chip changes it: the comparison "knapp" instead.
    await user.click(screen.getByRole("button", { name: /Frist\s*ist überschritten/ }));
    await user.keyboard("knapp{Enter}");
    expect(await screen.findByRole("row", { name: /Probe 000002/ })).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /Probe 000001/ })).toBeNull();
    query = state.calls.filter((c) => c.path === "/api/v1/records/probe.probe:query").at(-1)!;
    expect(JSON.parse(query.body!)).toMatchObject({
      where: {
        and: [
          { field: "sla_warn_at", lte: "@now" },
          { field: "sla_due_at", gt: "@now" },
        ],
      },
    });
  });

  it("reads breached and close back from a saved search", () => {
    const timed = { ...probeType, timed: true };
    for (const op of ["breached", "close"] as const) {
      const where = toWhere(timed, [{ field: "sla_due_at", op, value: "" }]);
      expect(fromWhere(where)).toEqual([{ field: "sla_due_at", op, value: "" }]);
    }
    const both = toWhere(timed, [
      { field: "sla_due_at", op: "close", value: "" },
      { field: "kind", op: "eq", value: "fault" },
    ]);
    expect(fromWhere(both)).toEqual([
      { field: "sla_due_at", op: "close", value: "" },
      { field: "kind", op: "eq", value: "fault" },
    ]);
  });

  it("estimates the share of the time from the three-quarter point", () => {
    const now = Date.parse("2026-11-02T12:00:00Z");
    const at = (m: number) => new Date(now + m * 60_000).toISOString();
    // 60 minutes, 30 left: half the time used.
    expect(deadlineOf({ sla_due_at: at(30), sla_warn_at: at(15) }, now)).toMatchObject({
      state: "running",
      share: 0.5,
    });
    expect(deadlineOf({ sla_due_at: at(10), sla_warn_at: at(-5) }, now)?.state).toBe("close");
    expect(deadlineOf({ sla_due_at: at(-1), sla_warn_at: at(-16) }, now)?.state).toBe("breached");
    expect(deadlineOf({ sla_paused: true }, now)?.state).toBe("paused");
    expect(deadlineOf({}, now)).toBeNull();
  });
});
