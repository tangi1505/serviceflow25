// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// A note of the workplace (SD9): the menu "Neu" of the list offers the phone note beside the
// form with all fields; the requester is found by search, the fixed value goes along unseen,
// and the new record opens.
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

const anna = "01a15a3c-8545-7691-b780-a00000000001";
// The persons are searched by name, as in the reference picker's test.
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

describe("notes of the workplace", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(2);
    state.signedIn = true;
    state.extraTypes = [personType];
    state.persons = { [anna]: { id: anna, first_name: "Anna", last_name: "Berg" } };
    state.forms = [
      {
        id: "01a15a3c-8545-7691-b780-f00000000009",
        name: "Telefonnotiz",
        description: "Legt einen Vorgang mit der Quelle Telefon an.",
        target_type: "probe.probe",
        place: "workplace",
        definition: {
          fixed: { kind: "question" },
          sections: [
            {
              id: "anrufer",
              title: { de: "Anrufer" },
              questions: [
                { id: "anforderer", role: "requester", required: true },
                { id: "nummer", type: "text", label: { de: "Rückrufnummer" } },
              ],
            },
            {
              id: "anliegen",
              title: { de: "Anliegen" },
              questions: [{ id: "titel", field: "title" }],
            },
          ],
        },
      },
    ];
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("takes a phone note from the menu Neu", async () => {
    const user = userEvent.setup();
    const { router, container } = renderAt("/types/probe.probe");
    // The forms load after the list: then "Neu" is a menu button (expanded false).
    await user.click(await screen.findByRole("button", { name: "Neu", expanded: false }));
    const menu = screen.getByRole("menu", { name: "Neu anlegen" });
    expect(within(menu).getByRole("menuitem", { name: /alle Felder/ })).toHaveFocus();
    await user.click(within(menu).getByRole("menuitem", { name: "Telefonnotiz" }));

    await screen.findByRole("heading", { name: "Telefonnotiz" });
    expect(router.state.location.pathname).toBe("/forms/01a15a3c-8545-7691-b780-f00000000009");
    const anrufer = screen.getByRole("group", { name: "Anrufer" });
    await user.type(within(anrufer).getByRole("combobox", { name: /Anforderer/ }), "Berg");
    await user.click(await screen.findByRole("option", { name: "Berg" }));
    await user.type(within(anrufer).getByLabelText("Rückrufnummer"), "0211 4711");
    await user.type(screen.getByLabelText(/Titel/), "Fachverfahren startet nicht");
    await expectAccessible(container);
    await user.click(screen.getByRole("button", { name: "Absenden" }));

    await waitFor(() =>
      expect(router.state.location.pathname).toMatch(/^\/types\/probe\.probe\/.+/),
    );
    const post = state.calls.find((c) => c.method === "POST" && c.path.endsWith(":submit"))!;
    expect(JSON.parse(post.body!)).toEqual({
      answers: { anforderer: anna, nummer: "0211 4711", titel: "Fachverfahren startet nicht" },
    });
    const made = state.records.at(-1)!;
    expect(made["kind"]).toBe("question");
    expect(made["actors"]).toEqual({ requester: [{ type: "core.person", id: anna }] });
  });

  it("keeps the plain button without forms of the workplace", async () => {
    state.forms = state.forms!.map((f) => ({ ...f, place: "portal" }));
    renderAt("/types/probe.probe");
    expect(await screen.findByRole("button", { name: "Neu" })).toBeInTheDocument();
    expect(screen.queryByRole("menu", { name: "Neu anlegen" })).toBeNull();
  });
});
