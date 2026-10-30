// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Tenants (PL44): an account of the installation switches the tenant in the header and starts
// at home; an account of one tenant sees no switch.
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
    const select = await screen.findByRole("combobox", { name: "Mandant" });
    expect(select).toHaveValue("root.nord");
    expect(screen.getByRole("option", { name: "Polizei Süd" })).toBeInTheDocument();
    await expectAccessible(view.container);

    await user.selectOptions(select, "root.sued");
    expect(await screen.findByRole("heading", { name: "Willkommen, Pascal" })).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe("/");
    expect(screen.getByRole("combobox", { name: "Mandant" })).toHaveValue("root.sued");
    const post = state.calls.find((c) => c.path === "/api/v1/auth/context");
    expect(post?.body).toBe(JSON.stringify({ tenant: "root.sued" }));
  });

  it("shows no switch with one tenant", async () => {
    state.tenants = [{ path: "root.nord", label: { de: "Polizei Nord" } }];
    renderAt("/");
    expect(await screen.findByRole("heading", { name: "Willkommen, Pascal" })).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Mandant" })).not.toBeInTheDocument();
  });
});
