// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The type and field editor (UI25, E29): the button at the list only with the right
// customize; a new field of its own lands in the customer layer and stands in list and form
// without a reload; hiding, moving and taking back are one change each, with the version seen;
// another version is reported and loaded afresh; a refusal of the server says why.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
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

// sent are the changes the editor posted, in order.
function sent(state: FakeState) {
  return state.calls
    .filter((c) => c.method === "POST" && c.path.endsWith("/customer"))
    .map((c) => JSON.parse(c.body ?? "{}") as { version: number; change: { op: string } });
}

describe("Type editor", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(2);
    state.signedIn = true;
    state.customize = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("offers the editor only with the right customize", async () => {
    state.customize = false;
    renderAt("/types/probe.probe");
    await screen.findByRole("heading", { name: "Probe" });
    // The editors sit in the menu "Anpassen" of the head (UI47), which is not there at all.
    expect(screen.queryByRole("button", { name: "Anpassen" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: /Typ anpassen/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Typen anpassen" })).not.toBeInTheDocument();
  });

  it("adds a field that shows in the list and the form at once", async () => {
    const user = userEvent.setup();
    const view = renderAt("/types/probe.probe");
    await screen.findByRole("heading", { name: "Probe" });
    // The menu "Anpassen" opens with the first entry focused and leads to the editor (UI47).
    await user.click(screen.getByRole("button", { name: "Anpassen" }));
    const menu = screen.getByRole("menu", { name: "Anpassen" });
    expect(screen.getByRole("menuitem", { name: /Typ anpassen/ })).toHaveFocus();
    await expectAccessible(view.container);
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: /Workflow/ })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(menu).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Anpassen" })).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "Anpassen" }));
    await user.click(screen.getByRole("menuitem", { name: /Typ anpassen/ }));
    await screen.findByRole("heading", { name: "Typ anpassen: Probe" });
    await expectAccessible(view.container);

    const form = screen.getByRole("region", { name: "Neues Feld" });
    await user.type(within(form).getByLabelText("Beschriftung"), "Dienststelle");
    expect(within(form).getByLabelText("Technischer Name")).toHaveValue("k_dienststelle");
    await user.click(within(form).getByRole("button", { name: "Feld an Probe anlegen" }));
    expect(await screen.findByText("Feld „Dienststelle“ angelegt.")).toBeInTheDocument();
    expect(sent(state)).toEqual([
      {
        version: 0,
        change: {
          op: "add_field",
          field: "k_dienststelle",
          new: {
            name: "k_dienststelle",
            type: "text",
            label: { de: "Dienststelle" },
            max_length: 200,
          },
        },
      },
    ]);
    // The field stands in the editor and the change below it, with who made it.
    const fields = screen.getByRole("region", { name: "Felder" });
    expect(
      await within(fields).findByRole("listitem", { name: "Dienststelle" }),
    ).toBeInTheDocument();
    const changes = screen.getByRole("region", { name: "Änderungen" });
    expect(
      within(changes).getByRole("row", {
        name: /Feld „Dienststelle“ \(k_dienststelle\) angelegt Pascal/,
      }),
    ).toBeInTheDocument();

    // In the list as a column and in the form of a new record, without a reload.
    await user.click(screen.getByRole("link", { name: "Zur Liste" }));
    expect(await screen.findByRole("columnheader", { name: /Dienststelle/ })).toBeInTheDocument();
    view.unmount();
    renderAt("/types/probe.probe/new");
    expect(await screen.findByLabelText(/^Dienststelle/)).toBeInTheDocument();
  });

  it("hides, moves and takes back, one change each", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe/customize");
    await screen.findByRole("heading", { name: "Typ anpassen: Probe" });
    const fields = screen.getByRole("region", { name: "Felder" });

    await user.click(within(fields).getByRole("button", { name: "„Aktiv“ ausblenden" }));
    expect(await screen.findByText("„Aktiv“ ausgeblendet.")).toBeInTheDocument();
    await user.click(within(fields).getByRole("button", { name: "„Fällig am“ nach oben" }));
    expect(
      await screen.findByText("„Fällig am“ steht jetzt hinter „Priorität“."),
    ).toBeInTheDocument();
    expect(sent(state).map((s) => [s.version, s.change])).toEqual([
      [0, { op: "hide_field", field: "active" }],
      [1, { op: "move_field", field: "due_on", after: "priority" }],
    ]);
    const names = within(fields)
      .getAllByRole("listitem")
      .map((li) => li.getAttribute("aria-label"));
    expect(names.slice(0, 4)).toEqual(["Titel", "Priorität", "Fällig am", "Aktiv"]);

    // Taking the hiding back shows the field again.
    const changes = screen.getByRole("region", { name: "Änderungen" });
    await user.click(
      within(changes).getByRole("button", { name: "Zurücknehmen: „Aktiv“ ausgeblendet" }),
    );
    expect(await screen.findByText("Zurückgenommen: „Aktiv“ ausgeblendet.")).toBeInTheDocument();
    expect(state.calls.some((c) => c.method === "DELETE" && c.query.get("version") === "2")).toBe(
      true,
    );
    expect(within(fields).getByRole("button", { name: "„Aktiv“ ausblenden" })).toBeInTheDocument();
  });

  it("changes the label of a field of the package and adds values", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe/customize");
    await screen.findByRole("heading", { name: "Typ anpassen: Probe" });
    const fields = screen.getByRole("region", { name: "Felder" });
    await user.click(within(fields).getByRole("button", { name: "„Art“ bearbeiten" }));
    const row = within(fields).getByRole("listitem", { name: "Art" });
    await user.type(within(row).getByLabelText("Weitere Werte, je Zeile einer"), "Hinweis");
    await user.click(within(row).getByRole("button", { name: "Werte ergänzen" }));
    expect(await screen.findByText("Werte für „Art“ ergänzt: Hinweis.")).toBeInTheDocument();

    await user.click(within(fields).getByRole("button", { name: "„Art“ bearbeiten" }));
    const again = within(fields).getByRole("listitem", { name: "Art" });
    const label = within(again).getByLabelText("Beschriftung");
    await user.clear(label);
    await user.type(label, "Anliegen");
    await user.click(within(again).getByRole("button", { name: "Speichern" }));
    expect(await screen.findByText("„Anliegen“ geändert.")).toBeInTheDocument();
    expect(sent(state).map((s) => s.change)).toEqual([
      {
        op: "add_options",
        field: "kind",
        options: [{ value: "hinweis", label: { de: "Hinweis" } }],
      },
      { op: "change_field", field: "kind", set: { label: { de: "Anliegen" } } },
    ]);
    expect(within(fields).getByRole("listitem", { name: "Anliegen" })).toBeInTheDocument();
  });

  it("reports a change made in between and a refusal of the server", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe/customize");
    await screen.findByRole("heading", { name: "Typ anpassen: Probe" });
    // Someone else hides a field meanwhile: version 1 on the server.
    state.customer!["probe.probe"]!.changes.push({ id: "x", op: "hide_field", field: "due_on" });
    state.customer!["probe.probe"]!.version = 1;
    const fields = screen.getByRole("region", { name: "Felder" });
    await user.click(within(fields).getByRole("button", { name: "„Aktiv“ ausblenden" }));
    expect(
      await screen.findByText(/Jemand anders hat den Typ gerade geändert/),
    ).toBeInTheDocument();
    // Loaded afresh: the hidden field shows as hidden now, and the next try goes through.
    expect(
      await within(fields).findByRole("button", { name: "„Fällig am“ einblenden" }),
    ).toBeInTheDocument();
    await user.click(within(fields).getByRole("button", { name: "„Aktiv“ ausblenden" }));
    expect(await screen.findByText("„Aktiv“ ausgeblendet.")).toBeInTheDocument();
    expect(sent(state).map((s) => s.version)).toEqual([0, 1]);

    // A field of the package has no "remove"; what the server refuses comes with its reason.
    expect(within(fields).queryByRole("button", { name: "„Titel“ entfernen" })).toBeNull();
    await user.click(within(fields).getByRole("button", { name: "„Titel“ ausblenden" }));
    expect(
      await screen.findByText(
        "Nicht übernommen: title is the title of probe.probe and stays visible",
      ),
    ).toBeInTheDocument();
  });

  it("lists the types with their changes in the overview", async () => {
    const user = userEvent.setup();
    state.customer = {
      "probe.probe": {
        type: "probe.probe",
        version: 1,
        changes: [{ id: "c1", op: "hide_field", field: "active", status: "active" }],
      },
    };
    const view = renderAt("/");
    await user.click(await screen.findByRole("link", { name: "Typen anpassen" }));
    await screen.findByRole("heading", { name: "Typen anpassen" });
    // The name of the row joins the link and the technical name without a space.
    expect(
      await screen.findByRole("row", { name: /^Probe\s?probe\.probe 1$/ }),
    ).toBeInTheDocument();
    await expectAccessible(view.container);
  });
});
