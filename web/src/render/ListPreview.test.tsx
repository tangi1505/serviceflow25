// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The frame and the list after the design (UI30, UI-Plan E35): the row height is compact or
// normal and kept with the person; a click on a row shows the record in the preview beside the
// list when there is room, "Öffnen" leads to the record page, on a narrow screen the click opens
// it at once; the search field of the head finds records over every type.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
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

function setWidth(px: number) {
  Object.defineProperty(window, "innerWidth", { value: px, configurable: true, writable: true });
}

describe("List and preview", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    state.extraTypes = [personType];
    state.persons = { [anna]: { id: anna, first_name: "Anna", last_name: "Berg" } };
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
    setWidth(1280);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    setWidth(1024);
  });

  it("switches the row height and keeps it with the person", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe");
    const row = await screen.findByRole("row", { name: /Probe 000001/ });
    expect(row).toHaveStyle({ height: "52px" });
    // The row height sits in the menu "Ansicht" of the bar (UI48).
    await user.click(screen.getByRole("button", { name: "Ansicht" }));
    const density = screen.getByRole("group", { name: "Zeilenhöhe" });
    expect(within(density).getByRole("menuitemradio", { name: "Normal" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await user.click(within(density).getByRole("menuitemradio", { name: "Kompakt" }));
    expect(
      await screen.findByRole("menuitemradio", { name: "Kompakt", checked: true }),
    ).toBeInTheDocument();
    expect(screen.getByRole("row", { name: /Probe 000001/ })).toHaveStyle({ height: "36px" });
    expect(state.settings).toMatchObject({ density: "compact" });
  });

  it("shows the chosen record beside the list and opens it", async () => {
    const user = userEvent.setup();
    const view = renderAt("/types/probe.probe");
    await user.click(await screen.findByRole("row", { name: /Probe 000002/ }));
    const preview = await screen.findByRole("complementary", { name: "Vorschau" });
    expect(
      await within(preview).findByRole("heading", { name: "Probe 000002" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("row", { name: /Probe 000002/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    // The state and the first fields; the priority as a mark only with format priority.
    expect(within(preview).getByText("Neu")).toBeInTheDocument();
    expect(within(preview).getByText("Priorität")).toBeInTheDocument();
    await expectAccessible(view.container);
    // Another row changes the preview; the list stays.
    await user.click(screen.getByRole("row", { name: /Probe 000003/ }));
    expect(
      await within(screen.getByRole("complementary", { name: "Vorschau" })).findByRole("heading", {
        name: "Probe 000003",
      }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("link", { name: "Öffnen" }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "Probe 000003" }),
    ).toBeInTheDocument();
  });

  it("closes the preview, and opens the record at once on a narrow screen", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe");
    await user.click(await screen.findByRole("row", { name: /Probe 000001/ }));
    await user.click(await screen.findByRole("button", { name: "Vorschau schließen" }));
    expect(screen.queryByRole("complementary", { name: "Vorschau" })).toBeNull();
    setWidth(800);
    await user.click(screen.getByRole("row", { name: /Probe 000001/ }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "Probe 000001" }),
    ).toBeInTheDocument();
  });

  // Since UI31 the search field of the head opens the command line (UI-Plan E36); its last
  // option leads to every hit on the search page.
  it("finds records over every type from the search field of the head", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe");
    await screen.findByRole("row", { name: /Probe 000001/ });
    await user.click(screen.getByRole("button", { name: /Suchen oder Befehl/ }));
    await user.keyboard("ber");
    await user.click(
      await screen.findByRole("option", { name: "Alle Treffer für „ber“ in der Suche" }),
    );
    expect(await screen.findByRole("heading", { name: "Suche: ber" })).toBeInTheDocument();
    const hit = await screen.findByRole("link", { name: "Berg" });
    expect(hit).toHaveAttribute("href", `/types/core.person/${anna}`);
  });
});
