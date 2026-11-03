// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The requester confirms or refuses a solution in the portal (SD2, SD-Plan E5, N8): the portal
// shows the transitions of the person's role, a refusal asks for its reason first; in the
// workplace an agent does not see the transitions of a role they do not hold.
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
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

const roleTransitions = (holder: boolean) => [
  {
    name: "confirm",
    label: { de: "Lösung bestätigen" },
    from: ["resolved"],
    to: "closed",
    by: ["requester"],
    holder,
  },
  {
    name: "refuse",
    label: { de: "Lösung ablehnen" },
    from: ["resolved"],
    to: "in_progress",
    by: ["requester"],
    comment: "required" as const,
    holder,
  },
];

describe("Confirming a solution", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(2);
    state.signedIn = true;
    state.records[0]!["state"] = "resolved";
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => vi.unstubAllGlobals());

  it("lets the requester refuse with a reason in the portal", async () => {
    state.workplace = false;
    state.roleTransitions = roleTransitions(true);
    const user = userEvent.setup();
    const id = String(state.records[0]!["id"]);
    const view = renderAt(`/portal/probe.probe/${id}`);
    const yours = await screen.findByRole("region", { name: "Ihre Rückmeldung" });
    expect(within(yours).getByRole("button", { name: "Lösung bestätigen" })).toBeEnabled();
    await user.click(within(yours).getByRole("button", { name: "Lösung ablehnen" }));
    const reason = within(yours).getByLabelText("Begründung");
    const send = within(yours)
      .getAllByRole("button", { name: "Lösung ablehnen" })
      .find((b) => b.getAttribute("type") === "submit")!;
    expect(send).toBeDisabled();
    await expectAccessible(view.container);
    await user.type(reason, "Er druckt immer noch leer.");
    await user.click(send);
    await waitFor(() => expect(state.records[0]!["state"]).toBe("in_progress"));
    expect(state.comments).toEqual(["Er druckt immer noch leer."]);
  });

  it("shows an agent no transition of a role they do not hold", async () => {
    state.roleTransitions = roleTransitions(false);
    const id = String(state.records[0]!["id"]);
    renderAt(`/types/probe.probe/${id}`);
    const box = await screen.findByRole("region", { name: "Status" });
    // The transitions of the type are there (disabled from this state), the role's are not.
    expect(await within(box).findByRole("button", { name: "Beginnen" })).toBeDisabled();
    expect(within(box).queryByRole("button", { name: "Lösung bestätigen" })).toBeNull();
  });
});
