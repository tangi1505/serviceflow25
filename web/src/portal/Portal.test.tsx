// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Portal and workplace (UI10): a user without workplace lands in the portal and does not
// reach the workplace, not by address either; creates with the fields of the portal and as
// requester; sees the record lean, with the timeline without "Privat"; an agent can switch.
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

const person = "01a15a3c-8545-7691-b780-a00000000077";

describe("Portal", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(2);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps a user in the portal, also by address", async () => {
    state.workplace = false;
    const home = renderAt("/");
    await waitFor(() => expect(home.router.state.location.pathname).toBe("/portal"));
    expect(await screen.findByRole("heading", { name: "Meine Vorgänge" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Zum Arbeitsplatz" })).toBeNull();
    home.unmount();
    const list = renderAt("/types/probe.probe");
    await waitFor(() => expect(list.router.state.location.pathname).toBe("/portal"));
  });

  it("creates as requester with the fields of the portal and answers in the timeline", async () => {
    const user = userEvent.setup();
    state.workplace = false;
    state.personId = person;
    const { router, container } = renderAt("/portal");
    const section = await screen.findByRole("region", { name: "Probe" });
    expect(await within(section).findByText("Noch nichts angelegt.")).toBeInTheDocument();
    await user.click(within(section).getByRole("link", { name: "Neu" }));
    await screen.findByRole("heading", { name: "Probe anlegen" });
    expect(screen.getByLabelText(/Titel/)).toBeInTheDocument();
    expect(screen.getByLabelText("Art")).toBeInTheDocument();
    expect(screen.queryByLabelText("Priorität")).toBeNull();
    expect(screen.queryByRole("group", { name: "Rollen am Datensatz" })).toBeNull();
    await user.type(screen.getByLabelText(/Titel/), "Drucker im Flur");
    await user.click(screen.getByRole("button", { name: "Anlegen" }));

    await screen.findByRole("heading", { name: "Drucker im Flur" });
    expect(router.state.location.pathname).toMatch(/^\/portal\/probe\.probe\//);
    const post = state.calls.find(
      (c) => c.method === "POST" && c.path === "/api/v1/records/probe.probe",
    )!;
    expect(JSON.parse(post.body!)).toMatchObject({
      title: "Drucker im Flur",
      actors: { requester: [{ type: "core.person", id: person }] },
    });
    const timeline = screen.getByRole("region", { name: "Zeitleiste" });
    expect(within(timeline).queryByLabelText("Privat (nur für Berechtigte)")).toBeNull();
    await expectAccessible(container);

    await user.click(screen.getByRole("link", { name: "Meine Vorgänge" }));
    const mine = await screen.findByRole("region", { name: "Probe" });
    expect(await within(mine).findByRole("link", { name: "Drucker im Flur" })).toBeInTheDocument();
  });

  it("fills in a form: sections, required answers, the record with the free answers (UI11)", async () => {
    const user = userEvent.setup();
    state.workplace = false;
    state.personId = person;
    state.forms = [
      {
        id: "01a15a3c-8545-7691-b780-f00000000001",
        name: "Störung melden",
        description: "Für alles, was nicht geht.",
        target_type: "probe.probe",
        definition: {
          sections: [
            {
              id: "s1",
              title: { de: "Anliegen" },
              questions: [{ id: "q1", field: "title", label: { de: "Worum geht es?" } }],
            },
            {
              id: "s2",
              title: { de: "Ort" },
              questions: [
                { id: "q2", type: "text", label: { de: "Raum" }, required: true },
                {
                  id: "q3",
                  type: "choice",
                  label: { de: "Etage" },
                  options: [
                    { value: "eg", label: { de: "Erdgeschoss" } },
                    { value: "og1", label: { de: "1. OG" } },
                  ],
                },
              ],
            },
          ],
        },
      },
    ];
    const { router, container } = renderAt("/portal");
    const forms = await screen.findByRole("region", { name: "Formulare" });
    expect(within(forms).getByText("Für alles, was nicht geht.")).toBeInTheDocument();
    await user.click(within(forms).getByRole("link", { name: "Störung melden" }));
    await screen.findByRole("heading", { name: "Störung melden" });
    const anliegen = screen.getByRole("group", { name: "Anliegen" });
    const ort = screen.getByRole("group", { name: "Ort" });
    await user.type(within(anliegen).getByLabelText(/Worum geht es\?/), "Drucker im Flur");
    await user.selectOptions(within(ort).getByLabelText("Etage"), "og1");
    expect(screen.queryByRole("group", { name: "Rollen am Datensatz" })).toBeNull();
    await expectAccessible(container);

    // Raum is required: the problem stands at the question.
    await user.click(screen.getByRole("button", { name: "Absenden" }));
    expect(await within(ort).findByText("Bitte ausfüllen.")).toBeInTheDocument();
    await user.type(within(ort).getByLabelText(/Raum/), "2.14");
    await user.click(screen.getByRole("button", { name: "Absenden" }));

    await screen.findByRole("heading", { name: "Drucker im Flur" });
    expect(router.state.location.pathname).toMatch(/^\/portal\/probe\.probe\//);
    const post = state.calls.find((c) => c.method === "POST" && c.path.endsWith(":submit"))!;
    expect(JSON.parse(post.body!)).toEqual({
      answers: { q1: "Drucker im Flur", q2: "2.14", q3: "og1" },
    });
    const timeline = screen.getByRole("region", { name: "Zeitleiste" });
    expect(await within(timeline).findByText(/Raum: 2\.14/)).toBeInTheDocument();
  });

  // The form of UI12: B ("Welches Gerät?") only when "Ersatzgerät nötig?" is yes.
  const conditional = {
    id: "01a15a3c-8545-7691-b780-f00000000002",
    name: "Störung mit Ersatz",
    target_type: "probe.probe",
    definition: {
      title: { en: "Fault with spare" },
      sections: [
        {
          id: "s1",
          title: { de: "Anliegen", en: "Request" },
          questions: [
            { id: "q1", field: "title" },
            {
              id: "q2",
              type: "boolean" as const,
              label: { de: "Ersatzgerät nötig?", en: "Spare device needed?" },
            },
            {
              id: "q3",
              type: "text" as const,
              label: { de: "Welches Gerät?", en: "Which device?" },
              required: true,
              show_if: [{ question: "q2", op: "eq" as const, value: "true" }],
            },
          ],
        },
      ],
    },
  };

  it("shows a question only when its condition holds, and does not send it hidden (UI12)", async () => {
    const user = userEvent.setup();
    state.workplace = false;
    state.personId = person;
    state.forms = [conditional];
    renderAt(`/portal/forms/${conditional.id}`);
    const group = await screen.findByRole("group", { name: "Anliegen" });
    expect(within(group).queryByLabelText(/Welches Gerät\?/)).toBeNull();
    await user.click(within(group).getByLabelText("Ersatzgerät nötig?"));
    const which = await within(group).findByLabelText(/Welches Gerät\?/);
    await user.type(within(group).getByLabelText(/Titel/), "Laptop kaputt");
    await user.click(screen.getByRole("button", { name: "Absenden" }));
    expect(await within(group).findByText("Bitte ausfüllen.")).toBeInTheDocument();
    await user.type(which, "Laptop");
    // No again: B goes, and with it its answer.
    await user.click(within(group).getByLabelText("Ersatzgerät nötig?"));
    expect(within(group).queryByLabelText(/Welches Gerät\?/)).toBeNull();
    await user.click(screen.getByRole("button", { name: "Absenden" }));
    await screen.findByRole("heading", { name: "Laptop kaputt" });
    const post = state.calls.find((c) => c.method === "POST" && c.path.endsWith(":submit"))!;
    expect(JSON.parse(post.body!)).toEqual({ answers: { q1: "Laptop kaputt", q2: false } });
  });

  it("shows the form in English for an English person (UI12)", async () => {
    state.workplace = false;
    state.settings = { language: "en" };
    state.forms = [conditional];
    const { container } = renderAt("/portal");
    const forms = await screen.findByRole("region", { name: "Forms" });
    expect(within(forms).getByRole("link", { name: "Fault with spare" })).toBeInTheDocument();
    await userEvent.setup().click(within(forms).getByRole("link", { name: "Fault with spare" }));
    const group = await screen.findByRole("group", { name: "Request" });
    expect(within(group).getByLabelText("Spare device needed?")).toBeInTheDocument();
    // The title has no text of its own: the field's English label.
    expect(within(group).getByLabelText(/Title/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send" })).toBeInTheDocument();
    await expectAccessible(container);
  });

  it("lets an agent switch between workplace and portal", async () => {
    const user = userEvent.setup();
    const { router } = renderAt("/");
    await user.click(await screen.findByRole("link", { name: "Zum Portal" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/portal"));
    await user.click(await screen.findByRole("link", { name: "Zum Arbeitsplatz" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
  });
});
