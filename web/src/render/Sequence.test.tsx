// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The running number (PL49, Kern-Plan E63) in the interface: the list and the record page show
// it with its prefix, the form shows it read-only and never sends it, the filter takes it with
// the prefix.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { CustomerChange, Field } from "@/api/client";
import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, probeType, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

import { formatValue, isCalculated, toInput, toRecord } from "./fields";

function renderAt(path: string) {
  const queryClient = createQueryClient();
  const router = createAppRouter({ queryClient, history: memoryHistory(path) });
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

const number: Field = { name: "k_nummer", type: "sequence", label: { de: "Nummer" }, prefix: "K-" };

describe("Running number", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    localStorage.clear();
    state.customer = {
      "probe.probe": {
        type: "probe.probe",
        version: 1,
        changes: [
          { id: "n", op: "add_field", field: "k_nummer", new: number } as unknown as CustomerChange,
        ],
      },
    };
    state.records.forEach((r, i) => (r["k_nummer"] = i + 1));
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => vi.unstubAllGlobals());

  it("shows the number with its prefix in the list and read-only on the record", async () => {
    const user = userEvent.setup();
    const view = renderAt("/types/probe.probe");
    const row = await screen.findByRole("row", { name: /Probe 000002/ });
    expect(within(row).getByText("K-2")).toBeInTheDocument();
    await expectAccessible(view.container);
    await user.click(row);
    const field = await screen.findByLabelText("Nummer");
    expect(field).toHaveValue("K-2");
    expect(field).toHaveAttribute("readonly");
  });

  it("filters by the number with its prefix", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe");
    await screen.findByRole("row", { name: /Probe 000001/ });
    // The filter row (UI50): the number is typed as it is, with its prefix.
    await user.click(screen.getByRole("combobox", { name: "Filter setzen" }));
    await user.keyboard("nummer{Enter}K-3{Enter}");
    const query = await vi.waitFor(() => {
      const last = state.calls.filter((c) => c.path === "/api/v1/records/probe.probe:query").at(-1);
      expect(last).toBeDefined();
      return last!;
    });
    expect(JSON.parse(query.body!)).toMatchObject({ where: { field: "k_nummer", eq: "K-3" } });
  });

  it("is kept by the server and never sent", () => {
    expect(isCalculated(number)).toBe(true);
    expect(formatValue(number, 24817, "de")).toBe("K-24817");
    const type = { ...probeType, fields: [...probeType.fields, number] };
    expect(toInput(type, { k_nummer: 7 })["k_nummer"]).toBe("K-7");
    expect(toRecord(type, { title: "x", k_nummer: "K-7" })).not.toHaveProperty("k_nummer");
  });
});
