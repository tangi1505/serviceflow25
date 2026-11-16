// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The navigation (UI36, UI plan E41, E48): areas named and drawn by their package, to open and
// close, closed with the number of their entries, kept per account in the browser; the narrow
// bar with symbols shows the entries of an area beside it and goes back to wide.
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

describe("Navigation", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    localStorage.clear();
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("opens and closes the areas of the packages and keeps them closed", async () => {
    const user = userEvent.setup();
    const view = renderAt("/");
    const nav = await screen.findByRole("navigation", { name: "Typen" });
    // The area is named by its package (E48), not by a raw name.
    const area = await within(nav).findByRole("button", { name: "Probe" });
    expect(area).toHaveAttribute("aria-expanded", "true");
    expect(within(nav).getByRole("link", { name: "Probe" })).toHaveAttribute(
      "href",
      "/types/probe.probe",
    );
    expect(within(nav).getByRole("link", { name: "Start" })).toHaveAttribute("href", "/");
    expect(within(nav).getByRole("button", { name: "Dashboards" })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: "Neues Dashboard" })).toBeInTheDocument();
    await expectAccessible(view.container);

    await user.click(area);
    expect(area).toHaveAttribute("aria-expanded", "false");
    expect(area).toHaveTextContent("Probe1");
    expect(within(nav).queryByRole("link", { name: "Probe" })).not.toBeInTheDocument();
    view.unmount();

    // Again, the area stays closed for this account.
    renderAt("/");
    const again = await screen.findByRole("button", { name: "Probe" });
    await waitFor(() => expect(again).toHaveAttribute("aria-expanded", "false"));
  });

  it("narrows the bar to symbols, shows the entries of an area beside it and widens it again", async () => {
    const user = userEvent.setup();
    const view = renderAt("/");
    await user.click(await screen.findByRole("button", { name: "Leiste einklappen" }));
    await waitFor(() =>
      expect(
        state.calls.some(
          (c) =>
            c.path === "/api/v1/me/settings" &&
            c.method === "PATCH" &&
            c.body === JSON.stringify({ nav: "narrow" }),
        ),
      ).toBe(true),
    );
    // The narrow bar is the one navigation now, of symbols only.
    const symbol = await screen.findByRole("button", { name: "Probe", expanded: false });
    const rail = screen.getByRole("navigation", { name: "Typen" });
    expect(within(rail).queryByRole("link", { name: "Probe" })).not.toBeInTheDocument();
    await user.click(symbol);
    const beside = within(rail).getByRole("group", { name: "Probe" });
    expect(within(beside).getByRole("link", { name: "Probe" })).toHaveAttribute(
      "href",
      "/types/probe.probe",
    );
    await expectAccessible(view.container);
    await user.keyboard("{Escape}");
    expect(beside).not.toBeInTheDocument();
    expect(symbol).toHaveFocus();

    await user.click(symbol);
    await user.click(
      within(within(rail).getByRole("group", { name: "Probe" })).getByRole("link", {
        name: "Probe",
      }),
    );
    await waitFor(() => expect(view.router.state.location.pathname).toBe("/types/probe.probe"));
    expect(within(rail).queryByRole("group", { name: "Probe" })).not.toBeInTheDocument();

    await user.click(within(rail).getByRole("button", { name: "Leiste ausklappen" }));
    expect(await screen.findByRole("button", { name: "Leiste einklappen" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Probe" })).toHaveAttribute(
      "href",
      "/types/probe.probe",
    );
  });
});
