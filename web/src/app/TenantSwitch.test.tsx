// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Tenants (PL44): an account of the installation switches the tenant in the header and starts
// at home; an account of one tenant sees no switch. Since UI35 the tenant is a button with a
// menu.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
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

describe("Tenant switch", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("switches the tenant and starts at home", async () => {
    state.tenants = [
      { path: "root.nord", label: { de: "Polizei Nord", en: "Police North" } },
      { path: "root.sued", label: { de: "Polizei Süd" } },
    ];
    state.tenant = "root.nord";
    const user = userEvent.setup();
    const view = renderAt("/types/probe.probe");
    const button = await screen.findByRole("button", { name: "Mandant: Polizei Nord" });
    await user.click(button);
    const menu = screen.getByRole("menu", { name: "Mandant wechseln" });
    expect(screen.getByRole("menuitemradio", { name: "Polizei Nord" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    // The checked one has the focus; the arrow keys move it, Esc closes and gives it back.
    expect(screen.getByRole("menuitemradio", { name: "Polizei Nord" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitemradio", { name: "Polizei Süd" })).toHaveFocus();
    await expectAccessible(view.container);
    await user.keyboard("{Escape}");
    expect(menu).not.toBeInTheDocument();
    expect(button).toHaveFocus();

    await user.click(button);
    await user.click(screen.getByRole("menuitemradio", { name: "Polizei Süd" }));
    expect(await screen.findByRole("heading", { name: "Willkommen, Pascal" })).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe("/");
    expect(screen.getByRole("button", { name: "Mandant: Polizei Süd" })).toBeInTheDocument();
    const post = state.calls.find((c) => c.path === "/api/v1/auth/context");
    expect(post?.body).toBe(JSON.stringify({ tenant: "root.sued" }));
  });

  it("shows no switch with one tenant", async () => {
    state.tenants = [{ path: "root.nord", label: { de: "Polizei Nord" } }];
    renderAt("/");
    expect(await screen.findByRole("heading", { name: "Willkommen, Pascal" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Mandant/ })).not.toBeInTheDocument();
  });
});
