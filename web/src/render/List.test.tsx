// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The list from the definition: pages by key, only the visible rows in the DOM (virtual),
// sort on the key columns, column choice kept per type, a row opens the record.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
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

describe("List", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(250);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
    localStorage.clear();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads the first page by key and renders only a window of rows", async () => {
    const { container } = renderAt("/types/probe.probe");
    await screen.findByText("100+ Datensätze");
    const list = state.calls.filter((c) => c.path === "/api/v1/records/probe.probe");
    expect(list[0]?.query.get("limit")).toBe("100");
    expect(list[0]?.query.get("sort")).toBe("-updated_at");
    expect(list[0]?.query.get("after")).toBeNull();
    // jsdom reports no height, so the virtualizer renders its overscan only: far fewer than 100.
    const rows = container.querySelectorAll("tbody tr");
    expect(rows.length).toBeGreaterThan(0);
    expect(rows.length).toBeLessThan(100);
    expect(rows[0]).toHaveTextContent("Probe 000250");
    await expectAccessible(container);
  });

  it("sorts on key columns, hides columns and remembers the choice", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/types/probe.probe");
    await screen.findByText("100+ Datensätze");
    await user.click(screen.getByRole("button", { name: /Geändert/ }));
    await waitFor(() =>
      expect(
        state.calls
          .filter((c) => c.path === "/api/v1/records/probe.probe")
          .at(-1)
          ?.query.get("sort"),
      ).toBe("updated_at"),
    );
    expect(container.querySelector("tbody tr")).toHaveTextContent("Probe 000001");
    expect(screen.getByRole("columnheader", { name: /Geändert/ })).toHaveAttribute(
      "aria-sort",
      "ascending",
    );

    // Columns are chosen in the menu "Ansicht" of the bar (UI48); it stays open meanwhile.
    await user.click(screen.getByRole("button", { name: "Ansicht" }));
    await user.click(screen.getByRole("menuitemcheckbox", { name: "Priorität" }));
    expect(screen.getByRole("menuitemcheckbox", { name: "Priorität" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
    await waitFor(() =>
      expect(screen.queryByRole("columnheader", { name: "Priorität" })).toBeNull(),
    );
    const stored = JSON.parse(
      localStorage.getItem("itsm.list.probe.probe.columns") ?? "{}",
    ) as Record<string, boolean>;
    expect(stored["priority"]).toBe(false);
  });

  it("drops a remembered column the type no longer has", async () => {
    // A field the customer layer took back (UI25) is still in the browser's choice; the list
    // asks only for columns of the type, or the API would refuse it (422).
    localStorage.setItem(
      "itsm.list.probe.probe.columns",
      JSON.stringify({ k_dienststelle: true, priority: false }),
    );
    renderAt("/types/probe.probe");
    await screen.findByText("100+ Datensätze");
    const list = state.calls.filter((c) => c.path === "/api/v1/records/probe.probe");
    const fields = list[0]?.query.get("fields")?.split(",") ?? [];
    expect(fields).not.toContain("k_dienststelle");
    expect(fields).not.toContain("priority");
    expect(fields).toContain("title");
  });

  it("opens a record from its row", async () => {
    const user = userEvent.setup();
    const { router, container } = renderAt("/types/probe.probe");
    await screen.findByText("100+ Datensätze");
    const row = container.querySelector("tbody tr");
    expect(row).not.toBeNull();
    await user.click(row!);
    await waitFor(() =>
      expect(router.state.location.pathname).toBe(
        "/types/probe.probe/01a15a3c-8545-7691-b780-000000000250",
      ),
    );
    await screen.findByRole("heading", { name: "Probe 000250" });
  });
});
