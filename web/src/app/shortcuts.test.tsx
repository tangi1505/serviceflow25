// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The shortcuts of the frame (UI52): Strg B folds the side bar and keeps it, "?" and the button
// show the dialog with the keys of the page, Esc closes it and the focus goes back, "/" jumps
// into the filter row, "N" creates; single keys do nothing in a field and nothing when turned
// off, while Strg K still works.
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

describe("Shortcuts", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
    // A wide screen: the side bar stands beside the content (UI36).
    vi.stubGlobal(
      "matchMedia",
      (q: string) =>
        ({
          matches: q.includes("min-width"),
          media: q,
          addEventListener: () => undefined,
          removeEventListener: () => undefined,
        }) as unknown as MediaQueryList,
    );
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("folds the side bar, shows the dialog, jumps and creates", async () => {
    const user = userEvent.setup();
    const { router, container } = renderAt("/types/probe.probe");
    await screen.findByText("3 Datensätze");

    // Strg B folds the bar in and out and keeps it.
    await user.keyboard("{Control>}b{/Control}");
    await waitFor(() => expect(state.settings).toMatchObject({ nav: "narrow" }));
    expect(screen.getByRole("button", { name: "Leiste ausklappen" })).toBeInTheDocument();
    await user.keyboard("{Control>}b{/Control}");
    await waitFor(() => expect(state.settings).toMatchObject({ nav: "wide" }));

    // "?" shows the dialog with the keys of the page; Esc closes it, the focus goes back.
    const row = screen.getByRole("row", { name: /Probe 000001/ });
    row.focus();
    await user.keyboard("?");
    const dialog = screen.getByRole("dialog", { name: "Tastenkürzel" });
    expect(within(dialog).getByText("Neu anlegen")).toBeInTheDocument();
    expect(within(dialog).getByText("In die Filterzeile springen")).toBeInTheDocument();
    await expectAccessible(container);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() => expect(row).toHaveFocus());
    // The button in the header opens it too.
    await user.click(screen.getByRole("button", { name: "Tastenkürzel" }));
    expect(screen.getByRole("dialog", { name: "Tastenkürzel" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Schließen" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Tastenkürzel" })).toHaveFocus());

    // "/" jumps into the filter row; typed there, it is text.
    row.focus();
    await user.keyboard("/");
    const filter = screen.getByRole("combobox", { name: "Filter setzen" });
    expect(filter).toHaveFocus();
    await user.keyboard("a/n");
    expect(filter).toHaveValue("a/n");
    await user.keyboard("{Escape}{Escape}");

    // "N" creates a new record.
    row.focus();
    await user.keyboard("n");
    await waitFor(() => expect(router.state.location.pathname).toBe("/types/probe.probe/new"));
  });

  it("leaves single keys off when the person turned them off", async () => {
    state.settings = { shortcuts: "off" };
    const user = userEvent.setup();
    const { router } = renderAt("/types/probe.probe");
    await screen.findByText("3 Datensätze");
    screen.getByRole("row", { name: /Probe 000001/ }).focus();
    await user.keyboard("/n?");
    expect(screen.getByRole("combobox", { name: "Filter setzen" })).not.toHaveFocus();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(router.state.location.pathname).toBe("/types/probe.probe");
    // Strg K is no single key and still works.
    await user.keyboard("{Control>}k{/Control}");
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });
});
