// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The picker (UI28): a reference chosen by name with the keyboard alone from a type without
// searchable fields (filtered in the browser); a role filled through the full-text search and
// saved with the fields; roles that did not change are not sent; axe on the open list.
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

import { searchable } from "./RefPicker";

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
const ben = "01a15a3c-8545-7691-b780-a00000000002";

describe("RefPicker", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    state.extraTypes = [personType];
    state.persons = {
      [anna]: { id: anna, first_name: "Anna", last_name: "Berg" },
      [ben]: { id: ben, first_name: "Ben", last_name: "Brandt" },
    };
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("knows which types the search finds", () => {
    expect(searchable(personType)).toBe(true);
    expect(searchable({ ...personType, fields: [{ name: "x", type: "text" }] })).toBe(false);
  });

  it("chooses a reference with the keyboard and a role by search, and saves both", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/types/probe.probe/new");
    await screen.findByRole("heading", { name: "Neuer Datensatz" });
    await user.type(screen.getByLabelText(/Titel/), "Mit Bezug");

    // Übergeordnet points to probe.probe, which has no searchable field: the first 100,
    // filtered in the browser; arrow down and Enter pick.
    const parent = screen.getByRole("combobox", { name: "Übergeordnet" });
    await user.type(parent, "000002");
    const options = await screen.findByRole("listbox", { name: "Vorschläge" });
    expect(
      await within(options).findByRole("option", { name: "Probe 000002" }),
    ).toBeInTheDocument();
    // The text applies once it stood still for 200 ms.
    await waitFor(() =>
      expect(within(options).queryByRole("option", { name: "Probe 000001" })).toBeNull(),
    );
    expect(parent).toHaveAttribute("aria-expanded", "true");
    await expectAccessible(container);
    await user.keyboard("{ArrowDown}{Enter}");
    expect(parent).toHaveAttribute("aria-expanded", "false");
    expect(parent).toHaveValue("");
    expect(await screen.findByText("Probe 000002")).toBeInTheDocument();

    // The requester through the full-text search; the chip shows the full name; Escape closes.
    const requester = screen.getByRole("combobox", { name: "Anforderer" });
    await user.type(requester, "ber");
    await user.click(await screen.findByRole("option", { name: "Berg" }));
    expect(await screen.findByText("Anna Berg")).toBeInTheDocument();
    await user.type(requester, "bran");
    await screen.findByRole("option", { name: "Brandt" });
    await user.keyboard("{Escape}");
    expect(requester).toHaveAttribute("aria-expanded", "false");
    await user.clear(requester);

    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    await screen.findByRole("heading", { name: "Mit Bezug" });
    const post = state.calls.find(
      (c) => c.method === "POST" && c.path === "/api/v1/records/probe.probe",
    );
    expect(JSON.parse(post!.body!)).toMatchObject({
      parent: "01a15a3c-8545-7691-b780-000000000002",
      actors: { requester: [{ type: "core.person", id: anna }] },
    });
  });

  it("sends only the roles that changed, and removes a holder", async () => {
    const user = userEvent.setup();
    const rec = state.records[0]!;
    rec["actors"] = { requester: [{ type: "core.person", id: anna }] };
    renderAt(`/types/probe.probe/${String(rec["id"])}`);
    await screen.findByRole("heading", { name: "Probe 000001" });

    // A change of the title alone: no actors in the body.
    const title = screen.getByLabelText(/Titel/);
    await user.clear(title);
    await user.type(title, "Nur der Titel");
    await user.click(screen.getByRole("button", { name: "Speichern" }));
    await screen.findByText("Gespeichert.");
    const first = state.calls.filter((c) => c.method === "PATCH").at(-1)!;
    expect(JSON.parse(first.body!)).not.toHaveProperty("actors");

    // Removing Anna clears the role.
    const form = screen.getByRole("group", { name: "Rollen am Datensatz" });
    await within(form).findByText("Anna Berg");
    await user.click(within(form).getByRole("button", { name: "Entfernen" }));
    await user.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(state.calls.filter((c) => c.method === "PATCH")).toHaveLength(2));
    const second = state.calls.filter((c) => c.method === "PATCH").at(-1)!;
    expect(JSON.parse(second.body!)).toMatchObject({ actors: { requester: [] } });
  });
});
