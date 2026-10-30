// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The state model in the form (WF1): the current state, one button per transition, those
// that do not apply are disabled with the reason, a click moves the record.
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

describe("Transitions", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the state, moves the record and explains disabled transitions", async () => {
    const user = userEvent.setup();
    const rec = state.records[0]!;
    rec["priority"] = 5;
    const { container } = renderAt(`/types/probe.probe/${rec["id"]}`);
    const box = await screen.findByRole("region", { name: "Status" });
    expect(box).toHaveTextContent("Neu");
    const start = await screen.findByRole("button", { name: "Beginnen" });
    const finish = screen.getByRole("button", { name: "Erledigen" });
    expect(start).toBeEnabled();
    expect(finish).toBeDisabled();
    expect(finish).toHaveAttribute("title", "Nicht aus dem aktuellen Status möglich.");
    await expectAccessible(container);

    await user.click(start);
    await waitFor(() => expect(box).toHaveTextContent("In Arbeit"));
    expect(rec["state"]).toBe("in_progress");
    expect(await screen.findByText("Version 2")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Erledigen" }));
    await waitFor(() => expect(box).toHaveTextContent("Erledigt"));
    // priority 5: reopen is refused by its condition.
    const reopen = screen.getByRole("button", { name: "Wieder öffnen" });
    await waitFor(() => expect(reopen).toBeDisabled());
    expect(reopen).toHaveAttribute("title", "Die Bedingung ist nicht erfüllt.");
  });

  it("shows the server's refusal", async () => {
    const user = userEvent.setup();
    const rec = state.records[0]!;
    renderAt(`/types/probe.probe/${rec["id"]}`);
    const start = await screen.findByRole("button", { name: "Beginnen" });
    // Someone else moved the record in between: the click is refused with 409.
    rec["state"] = "done";
    await user.click(start);
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Übergang nicht möglich: start: state",
    );
    await waitFor(() => expect(screen.getByRole("button", { name: "Beginnen" })).toBeDisabled());
  });
});
