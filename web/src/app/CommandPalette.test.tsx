// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The command line (UI31, UI-Plan E36): Strg K opens it from anywhere; it finds records over
// every type, runs commands (new record, light or dark) and lists the records opened last; all
// of it with the keyboard only, Esc gives the focus back.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { TypeDefinition } from "@/api/client";
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

const personType: TypeDefinition = {
  name: "person",
  package: "core",
  label: { de: "Person" },
  title_field: "last_name",
  fields: [
    { name: "first_name", type: "text", searchable: true },
    { name: "last_name", type: "text", searchable: true },
  ],
};
const anna = "01a15a3c-8545-7691-b780-a00000000001";

describe("Command line", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    state.extraTypes = [personType];
    state.persons = { [anna]: { id: anna, first_name: "Anna", last_name: "Berg" } };
    localStorage.clear();
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    document.documentElement.dataset.colorScheme = "light";
  });

  it("finds a record over every type with Strg K and opens it with Enter", async () => {
    const user = userEvent.setup();
    const view = renderAt("/types/probe.probe");
    await screen.findByRole("row", { name: /Probe 000001/ });
    await user.keyboard("{Control>}k{/Control}");
    const dialog = await screen.findByRole("dialog", { name: "Befehlszeile" });
    const input = within(dialog).getByRole("combobox", { name: "Befehl oder Suche" });
    expect(input).toHaveFocus();
    await user.keyboard("berg");
    const records = await within(dialog).findByRole("group", { name: "Datensätze" });
    expect(within(records).getByRole("option", { name: /Berg/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expectAccessible(view.container);
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(view.router.state.location.pathname).toBe(`/types/core.person/${anna}`),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers a new record of a type and changes the appearance", async () => {
    const user = userEvent.setup();
    const view = renderAt("/types/probe.probe");
    await screen.findByRole("row", { name: /Probe 000001/ });
    await user.keyboard("{Control>}k{/Control}");
    await user.keyboard("neu probe");
    const commands = await screen.findByRole("group", { name: "Befehle" });
    expect(within(commands).getByRole("option", { name: "Neu: Probe" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await user.keyboard("{Enter}");
    await waitFor(() => expect(view.router.state.location.pathname).toBe("/types/probe.probe/new"));
    // Dark, kept with the person as in the settings.
    await user.keyboard("{Control>}k{/Control}");
    await user.keyboard("dunkel");
    expect(await screen.findByRole("option", { name: "Darstellung: Dunkel" })).toBeInTheDocument();
    await user.keyboard("{Enter}");
    await waitFor(() => expect(document.documentElement.dataset.colorScheme).toBe("dark"));
    expect(state.settings).toMatchObject({ color_scheme: "dark" });
  });

  it("lists the records opened last and gives the focus back on Esc", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe");
    // Below 1100 px the click on a row opens the record at once (UI30).
    await user.click(await screen.findByRole("row", { name: /Probe 000002/ }));
    await screen.findByRole("heading", { level: 1, name: "Probe 000002" });
    // From the search field of the head, with the arrows down to the recent record.
    const head = screen.getByRole("button", { name: /Suchen oder Befehl/ });
    await user.click(head);
    const recent = await screen.findByRole("group", { name: "Zuletzt geöffnet" });
    expect(within(recent).getByRole("option", { name: /Probe 000002/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await user.keyboard("{ArrowDown}");
    expect(within(recent).getByRole("option", { name: /Probe 000002/ })).toHaveAttribute(
      "aria-selected",
      "false",
    );
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(head).toHaveFocus();
  });
});
