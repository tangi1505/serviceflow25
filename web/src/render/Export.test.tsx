// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The export button (UI5): it sends the view of the list (condition, columns, sort, language,
// time zone) and saves the file; a large export shows its job and then the link to the file.
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

describe("Export", () => {
  let state: FakeState;
  const created = vi.fn(() => "blob:export");
  beforeEach(() => {
    state = newState(6);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
    URL.createObjectURL = created;
    URL.revokeObjectURL = vi.fn();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    created.mockClear();
  });

  it("sends the view of the list and saves the file", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/types/probe.probe");
    await screen.findByText("6 Datensätze");
    // The filter row (UI50): "Art ist Störung".
    await user.click(screen.getByRole("combobox", { name: "Filter setzen" }));
    await user.keyboard("art{Enter}stör{Enter}");
    await screen.findByText("2 Datensätze");
    // "Exportieren" is a menu of the formats in the bar (UI48); a format starts it.
    await user.click(screen.getByRole("button", { name: "Exportieren" }));
    await expectAccessible(container);
    await user.click(screen.getByRole("menuitem", { name: "CSV" }));
    await waitFor(() => expect(created).toHaveBeenCalledTimes(1));
    const call = state.calls.find((c) => c.path === "/api/v1/records/probe.probe:export")!;
    const body = JSON.parse(call.body!) as Record<string, unknown>;
    expect(body).toMatchObject({
      format: "csv",
      where: { field: "kind", eq: "fault" },
      sort: "-updated_at",
      lang: "de",
    });
    expect(body["columns"]).toEqual(expect.arrayContaining(["title", "kind", "updated_at"]));
    expect(typeof body["time_zone"]).toBe("string");
  });

  it("follows the job of a large export and offers its file", async () => {
    const user = userEvent.setup();
    state.exportJob = true;
    renderAt("/types/probe.probe");
    await screen.findByText("6 Datensätze");
    await user.click(screen.getByRole("button", { name: "Exportieren" }));
    await user.click(screen.getByRole("menuitem", { name: "Excel (XLSX)" }));
    expect(await screen.findByText(/läuft als Job/)).toBeInTheDocument();
    // While the job runs, no second export starts.
    await user.click(screen.getByRole("button", { name: "Exportieren" }));
    expect(screen.getByRole("menuitem", { name: "CSV" })).toBeDisabled();
    await user.keyboard("{Escape}");
    state.jobState = "completed";
    expect(
      await screen.findByText("Export fertig: 123456 Zeilen.", {}, { timeout: 5000 }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Datei herunterladen" })).toHaveAttribute(
      "href",
      "/api/v1/files/01a15a3c-8545-7691-b780-f11e00000001",
    );
    expect(created).not.toHaveBeenCalled();
  });
});
