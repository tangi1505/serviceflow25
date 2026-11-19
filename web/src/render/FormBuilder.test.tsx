// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The form builder (UI11): a form with three sections, bound and free questions, ordered with
// the buttons; saving sends the structure as JSON. Dragging with the keyboard needs real layout
// and is tested in the browser (e2e/forms.spec.ts).
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, formTypeDef, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";
import { selectOption } from "@/test/select";

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

// The longest tests of the web, keyboard by keyboard: under load they passed the default of
// 5 s now and then (PL52).
describe("FormBuilder", { timeout: 15_000 }, () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(0);
    state.signedIn = true;
    state.extraTypes = [formTypeDef];
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("builds three sections, orders them and saves the structure", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/types/core.form/new");
    await user.type(await screen.findByLabelText(/^Name/), "Störung melden");
    // Without a target the builder says what to do first.
    expect(screen.getByText(/Erst angeben, was das Formular anlegt/)).toBeInTheDocument();
    await user.type(screen.getByLabelText(/^Legt an/), "probe.probe");
    const builder = screen.getByRole("group", { name: "Aufbau" });

    // Section 1: the title of the record, bound to its field.
    const first = within(builder).getByRole("region", { name: "Abschnitt 1" });
    await user.type(within(first).getByLabelText("Titel des Abschnitts"), "Anliegen");
    const s1 = within(builder).getByRole("region", { name: "Anliegen" });
    await selectOption(user, within(s1).getByLabelText("Neue Frage"), "field:title");
    await user.click(within(s1).getByRole("button", { name: "Frage hinzufügen" }));
    expect(within(s1).getByText("Feld: Titel")).toBeInTheDocument();
    // A bound field is not offered twice.
    await user.click(within(s1).getByLabelText("Neue Frage"));
    expect(screen.queryByRole("option", { name: "Titel" })).toBeNull();
    await user.keyboard("{Escape}");

    // Section 2: a free text question, required.
    await user.click(within(builder).getByRole("button", { name: "Abschnitt hinzufügen" }));
    await user.type(
      within(within(builder).getByRole("region", { name: "Abschnitt 2" })).getByLabelText(
        "Titel des Abschnitts",
      ),
      "Ort",
    );
    const s2 = within(builder).getByRole("region", { name: "Ort" });
    await selectOption(user, within(s2).getByLabelText("Neue Frage"), "free:text");
    await user.click(within(s2).getByRole("button", { name: "Frage hinzufügen" }));
    await user.type(within(s2).getByLabelText("Frage"), "Raum");
    await user.click(within(s2).getByLabelText("Pflicht"));

    // Section 3: a choice with two answers.
    await user.click(within(builder).getByRole("button", { name: "Abschnitt hinzufügen" }));
    await user.type(
      within(within(builder).getByRole("region", { name: "Abschnitt 3" })).getByLabelText(
        "Titel des Abschnitts",
      ),
      "Etage",
    );
    const s3 = within(builder).getByRole("region", { name: "Etage" });
    await selectOption(user, within(s3).getByLabelText("Neue Frage"), "free:choice");
    await user.click(within(s3).getByRole("button", { name: "Frage hinzufügen" }));
    await user.type(within(s3).getByLabelText("Frage"), "Welche Etage?");
    await user.type(
      within(s3).getByLabelText("Antworten, je Zeile eine"),
      "Erdgeschoss{Enter}1. OG",
    );

    // Etage before Ort, with the buttons.
    await user.click(within(builder).getByRole("button", { name: "„Etage“ nach oben" }));
    const order = within(builder)
      .getAllByRole("region")
      .map((r) => r.getAttribute("aria-label"));
    expect(order).toEqual(["Anliegen", "Etage", "Ort"]);
    await expectAccessible(container);

    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    await waitFor(() =>
      expect(
        state.calls.some((c) => c.method === "POST" && c.path === "/api/v1/records/core.form"),
      ).toBe(true),
    );
    const post = state.calls.find(
      (c) => c.method === "POST" && c.path === "/api/v1/records/core.form",
    )!;
    const body = JSON.parse(post.body!) as { target_type: string; definition: string };
    expect(body.target_type).toBe("probe.probe");
    expect(JSON.parse(body.definition)).toEqual({
      sections: [
        { id: "s1", title: { de: "Anliegen" }, questions: [{ id: "q1", field: "title" }] },
        {
          id: "s3",
          title: { de: "Etage" },
          questions: [
            {
              id: "q3",
              type: "choice",
              label: { de: "Welche Etage?" },
              options: [
                { value: "erdgeschoss", label: { de: "Erdgeschoss" } },
                { value: "1_og", label: { de: "1. OG" } },
              ],
            },
          ],
        },
        {
          id: "s2",
          title: { de: "Ort" },
          questions: [{ id: "q2", type: "text", label: { de: "Raum" }, required: true }],
        },
      ],
    });
  });

  it("orders questions within a section and removes one", async () => {
    const user = userEvent.setup();
    renderAt("/types/core.form/new");
    await user.type(await screen.findByLabelText(/^Legt an/), "probe.probe");
    const builder = screen.getByRole("group", { name: "Aufbau" });
    const s1 = within(builder).getByRole("region", { name: "Abschnitt 1" });
    for (const pick of ["field:title", "field:kind", "free:boolean"]) {
      await selectOption(user, within(s1).getByLabelText("Neue Frage"), pick);
      await user.click(within(s1).getByRole("button", { name: "Frage hinzufügen" }));
    }
    const names = () =>
      within(s1)
        .getAllByRole("listitem")
        .map((li) => li.getAttribute("aria-label"));
    expect(names()).toEqual(["Titel", "Art", "q3"]);
    await user.click(within(s1).getByRole("button", { name: "„Art“ nach oben" }));
    expect(names()).toEqual(["Art", "Titel", "q3"]);
    await user.click(within(s1).getByRole("button", { name: "„Titel“ entfernen" }));
    expect(names()).toEqual(["Art", "q3"]);
    // The removed field can be asked again.
    await user.click(within(s1).getByLabelText("Neue Frage"));
    expect(screen.getByRole("option", { name: "Titel" })).toBeInTheDocument();
  });

  it("sets a condition on an earlier question and the English texts (UI12)", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/types/core.form/new");
    await user.type(await screen.findByLabelText(/^Name/), "Störung");
    await user.type(screen.getByLabelText(/^Legt an/), "probe.probe");
    const builder = screen.getByRole("group", { name: "Aufbau" });
    await user.type(
      within(within(builder).getByRole("region", { name: "Abschnitt 1" })).getByLabelText(
        "Titel des Abschnitts",
      ),
      "Anliegen",
    );
    const s1 = within(builder).getByRole("region", { name: "Anliegen" });
    for (const [pick, label] of [
      ["field:title", ""],
      ["free:boolean", "Ersatzgerät nötig?"],
      ["free:text", "Welches Gerät?"],
    ] as const) {
      await selectOption(user, within(s1).getByLabelText("Neue Frage"), pick);
      await user.click(within(s1).getByRole("button", { name: "Frage hinzufügen" }));
      if (label) {
        const rows = within(s1).getAllByRole("listitem");
        await user.type(within(rows[rows.length - 1]!).getByLabelText("Frage"), label);
      }
    }
    // The first question has nothing before it to refer to.
    const title = within(s1).getByRole("listitem", { name: "Titel" });
    expect(within(title).getByRole("button", { name: "Bedingung hinzufügen" })).toBeDisabled();
    const which = within(s1).getByRole("listitem", { name: "Welches Gerät?" });
    await user.click(within(which).getByRole("button", { name: "Bedingung hinzufügen" }));
    await selectOption(user, within(which).getByLabelText("Bezug auf Frage"), "q2");
    await selectOption(user, within(which).getByLabelText("Bedingung"), "eq");
    await selectOption(user, within(which).getByLabelText("Wert"), "true");
    expect(within(which).getByRole("group", { name: "Nur zeigen, wenn" })).toBeInTheDocument();

    // English: the same inputs now edit the English texts.
    await user.click(within(builder).getByRole("radio", { name: "Englisch" }));
    await user.type(
      within(builder).getByLabelText("Name des Formulars auf Englisch"),
      "Report a fault",
    );
    await user.type(within(s1).getByLabelText("Titel des Abschnitts (Englisch)"), "Request");
    await user.type(
      within(within(s1).getByRole("listitem", { name: "Ersatzgerät nötig?" })).getByLabelText(
        "Frage (Englisch)",
      ),
      "Spare device needed?",
    );
    await expectAccessible(container);

    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    await waitFor(() =>
      expect(
        state.calls.some((c) => c.method === "POST" && c.path === "/api/v1/records/core.form"),
      ).toBe(true),
    );
    const post = state.calls.find(
      (c) => c.method === "POST" && c.path === "/api/v1/records/core.form",
    )!;
    const def = JSON.parse((JSON.parse(post.body!) as { definition: string }).definition);
    expect(def.title).toEqual({ en: "Report a fault" });
    expect(def.sections[0].title).toEqual({ de: "Anliegen", en: "Request" });
    expect(def.sections[0].questions[1].label).toEqual({
      de: "Ersatzgerät nötig?",
      en: "Spare device needed?",
    });
    expect(def.sections[0].questions[2].show_if).toEqual([
      { question: "q2", op: "eq", value: "true" },
    ]);
  });
});
