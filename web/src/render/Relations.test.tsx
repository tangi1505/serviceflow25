// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The relations tab of the record page (SD4, UI-Plan E38): first of the tabs, the timeline
// last; a relation is linked by kind and record, shows from both sides with label or inverse,
// and is removed after asking; a kind may name a trait (N11).
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { TypeDefinition } from "@/api/client";
import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, probeType, type FakeState } from "@/test/fakeApi";
import { answer } from "@/test/confirm";
import { expectAccessible } from "@/test/setup";

import { relates } from "./Relations";
import { selectOption } from "@/test/select";

function renderAt(path: string) {
  const queryClient = createQueryClient();
  const router = createAppRouter({ queryClient, history: memoryHistory(path) });
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
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

describe("Relations", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(2);
    state.signedIn = true;
    state.extraTypes = [personType];
    state.persons = { [anna]: { id: anna, first_name: "Anna", last_name: "Berg" } };
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => vi.unstubAllGlobals());

  it("knows the types a side takes, traits too", () => {
    const ticket = { ...probeType, package: "incident", name: "incident", traits: ["sd.ticket"] };
    const own = { ...probeType, package: "sd", name: "thing", traits: ["ticket"] };
    expect(relates(["trait:sd.ticket"], ticket)).toBe(true);
    expect(relates(["trait:sd.ticket"], own)).toBe(true);
    expect(relates(["trait:sd.ticket"], probeType)).toBe(false);
    expect(relates(["*"], probeType)).toBe(true);
    expect(relates(["probe.probe"], probeType)).toBe(true);
  });

  it("links, shows both directions and removes after asking", async () => {
    const user = userEvent.setup();
    const [one, two] = state.records.map((r) => String(r["id"]));
    // The second record names the first as its duplicate: here it reads "hat Duplikat".
    state.relations = [
      {
        id: "rel-0",
        kind: "core.duplicate_of",
        from: { type: "probe.probe", id: two! },
        to: { type: "probe.probe", id: one! },
        created_at: "2026-11-03T10:00:00Z",
      },
    ];
    const view = renderAt(`/types/probe.probe/${one}`);
    const tabs = await screen.findByRole("tablist", { name: "Bereiche des Datensatzes" });
    // Since SD16b a type with history has "Fassungen" between them.
    const [first, middle, last] = within(tabs).getAllByRole("tab");
    expect(first).toHaveAccessibleName("Bezüge");
    expect(first).toHaveAttribute("aria-selected", "true");
    expect(middle).toHaveAccessibleName("Fassungen");
    expect(last).toHaveAccessibleName("Zeitleiste");
    const panel = screen.getByRole("tabpanel", { name: "Bezüge" });
    const table = await within(panel).findByRole("table");
    expect(await within(table).findByRole("link", { name: "Probe 000002" })).toBeInTheDocument();
    expect(table).toHaveTextContent("hat Duplikat");

    // Linked to Anna as "verknüpft mit", found by name.
    await selectOption(user, within(panel).getByLabelText("Art"), "verknüpft mit");
    await user.type(within(panel).getByRole("combobox", { name: "Datensatz" }), "ber");
    await user.click(await screen.findByRole("option", { name: "Berg" }));
    await expectAccessible(view.container);
    await user.click(within(panel).getByRole("button", { name: "Verknüpfen" }));
    await waitFor(() => expect(state.relations).toHaveLength(2));
    expect(state.relations![1]).toMatchObject({
      kind: "core.related",
      from: { type: "probe.probe", id: one },
      to: { type: "core.person", id: anna },
    });
    expect(await within(panel).findByRole("link", { name: "Anna Berg" })).toBeInTheDocument();

    // Removing asks first; cancel keeps it, the second time it goes.
    const remove = () =>
      within(panel).getByRole("button", { name: "Bezug „verknüpft mit“ zu Anna Berg lösen" });
    await user.click(remove());
    await answer(user, "Abbrechen");
    expect(state.relations).toHaveLength(2);
    await user.click(remove());
    await answer(user, "Lösen");
    await waitFor(() => expect(state.relations).toHaveLength(1));
    await waitFor(() =>
      expect(within(panel).queryByRole("link", { name: "Anna Berg" })).toBeNull(),
    );

    // The arrow key moves to the versions (SD16b), End to the timeline.
    first!.focus();
    await user.keyboard("{ArrowRight}");
    expect(middle).toHaveFocus();
    await user.keyboard("{End}");
    expect(last).toHaveFocus();
    expect(await screen.findByRole("region", { name: "Zeitleiste" })).toBeInTheDocument();
  });
});
