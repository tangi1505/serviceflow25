// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The role editor (UI27, UI-Plan E33): a role is made in the overview; in the matrix a cell's
// scope is set, changed and taken away, each one call that holds at once; the details of a
// cell narrow it by condition and fields and add a further scope; a special right is a box;
// a person gets the role at a node and loses it again.
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

// calls are the changes the editor sent, as method, path and body.
function calls(state: FakeState) {
  return state.calls
    .filter(
      (c) =>
        c.method !== "GET" &&
        (c.path.startsWith("/api/v1/roles") || c.path.startsWith("/api/v1/assignments")),
    )
    .map((c) => {
      const scope = c.query.get("scope");
      return {
        call: `${c.method} ${c.path}${scope ? `?scope=${scope}` : ""}`,
        body: c.body ? (JSON.parse(c.body) as unknown) : undefined,
      };
    });
}

describe("Role editor", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.manageRights = true;
    state.extraTypes = [personType];
    state.persons = { [anna]: { id: anna, first_name: "Anna", last_name: "Berg" } };
    state.roles = [
      {
        id: "role-agent",
        name: "agent",
        label: { de: "Bearbeiter" },
        description: {},
        permissions: [{ type_name: "probe.probe", action: "read", scope: "all" }],
      },
    ];
    state.assignments = [];
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("makes a role from the overview and opens it", async () => {
    const user = userEvent.setup();
    const view = renderAt("/");
    await user.click(await screen.findByRole("link", { name: "Rollen und Rechte" }));
    await screen.findByRole("heading", { name: "Rollen" });
    // The overview in the look of the list (UI54): the number under the title, a field that
    // narrows the rows, a whole row that opens with Enter; "Neue Rolle" opens the form.
    const rows = () => screen.getAllByRole("row").filter((r) => r.closest("tbody"));
    expect(screen.getByText("1 Rolle")).toBeInTheDocument();
    expect(within(rows()[0]!).getByText("Bearbeiter")).toBeInTheDocument();
    const search = screen.getByRole("searchbox", { name: "Rollen durchsuchen" });
    await user.type(search, "AGEN");
    expect(rows()).toHaveLength(1);
    await user.type(search, "x");
    expect(rows()).toHaveLength(0);
    expect(screen.getByText("Keine Rolle passt zur Suche.")).toBeInTheDocument();
    await user.clear(search);
    await expectAccessible(view.container);
    await user.click(screen.getByRole("button", { name: "Neue Rolle" }));
    const form = screen.getByRole("form", { name: "Neue Rolle" });
    expect(within(form).getByLabelText("Beschriftung")).toHaveFocus();
    await user.type(within(form).getByLabelText("Beschriftung"), "Prüfer");
    await user.type(within(form).getByLabelText("Technischer Name"), "pruefer");
    await user.click(within(form).getByRole("button", { name: "Rolle anlegen" }));
    expect(await screen.findByRole("heading", { name: "Rolle: Prüfer" })).toBeInTheDocument();
    expect(calls(state)).toEqual([
      { call: "POST /api/v1/roles", body: { name: "pruefer", label: { de: "Prüfer" } } },
    ]);
  });

  it("sets, changes and takes away the scope of a cell, each at once", async () => {
    const user = userEvent.setup();
    const view = renderAt("/roles/agent");
    await screen.findByRole("heading", { name: "Rolle: Bearbeiter" });
    const read = screen.getByRole("combobox", { name: "Probe: lesen" });
    // Deleting for good is a right of its own, a column of the matrix (PL75).
    expect(screen.getByRole("combobox", { name: "Probe: endgültig löschen" })).toBeInTheDocument();
    expect(read).toHaveAttribute("data-value", "all");
    expect(screen.getByRole("combobox", { name: "Probe: ändern" })).toHaveAttribute(
      "data-value",
      "",
    );
    await expectAccessible(view.container);

    // Narrowing read from all to own: the new line, then the old one goes.
    await selectOption(user, read, "own");
    expect(await screen.findByText("Probe: lesen: eigene.")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Probe: lesen" })).toHaveAttribute(
      "data-value",
      "own",
    );
    await selectOption(user, screen.getByRole("combobox", { name: "Probe: ändern" }), "group");
    expect(await screen.findByText("Probe: ändern: Gruppe.")).toBeInTheDocument();
    await selectOption(user, screen.getByRole("combobox", { name: "Probe: lesen" }), "");
    expect(await screen.findByText("Probe: lesen: kein Recht mehr.")).toBeInTheDocument();

    expect(calls(state)).toEqual([
      {
        call: "PUT /api/v1/roles/agent/permissions",
        body: { type_name: "probe.probe", action: "read", scope: "own" },
      },
      {
        call: "DELETE /api/v1/roles/agent/permissions/probe.probe/read?scope=all",
        body: undefined,
      },
      {
        call: "PUT /api/v1/roles/agent/permissions",
        body: { type_name: "probe.probe", action: "update", scope: "group" },
      },
      { call: "DELETE /api/v1/roles/agent/permissions/probe.probe/read", body: undefined },
    ]);
    // Only types with rights.
    await user.click(screen.getByRole("checkbox", { name: "Nur Typen mit Rechten" }));
    expect(screen.getByRole("rowheader", { name: "Probe" })).toBeInTheDocument();
    expect(screen.queryByRole("rowheader", { name: "Person" })).toBeNull();
  });

  it("narrows a cell by condition and fields and adds a further scope", async () => {
    const user = userEvent.setup();
    const view = renderAt("/roles/agent");
    await screen.findByRole("heading", { name: "Rolle: Bearbeiter" });
    await user.click(screen.getByRole("button", { name: "Details: Probe: lesen" }));
    const details = screen.getByRole("region", { name: "Probe: lesen" });
    const line = within(details).getByRole("form", { name: "Probe: lesen, alle" });

    // A condition that does not compile is refused with the reason.
    const when = within(line).getByLabelText("Bedingung");
    await user.type(when, "record.priority >");
    await user.click(within(line).getByRole("button", { name: "Speichern" }));
    expect(await screen.findByText(/Syntax error/)).toBeInTheDocument();

    await user.type(when, " 1");
    await user.click(within(line).getByRole("checkbox", { name: "Alle Felder" }));
    await user.click(within(line).getByRole("checkbox", { name: "Fällig am" }));
    await user.click(within(line).getByRole("button", { name: "Speichern" }));
    expect(await screen.findByText("Probe: lesen, alle gespeichert.")).toBeInTheDocument();
    await expectAccessible(view.container);

    // A further scope for the same cell.
    await user.click(within(details).getByRole("button", { name: "Weitere Reichweite" }));
    const extra = within(details).getByRole("form", { name: "Probe: lesen, eigene" });
    await selectOption(user, within(extra).getByLabelText("Reichweite"), "involved");
    await user.click(within(extra).getByRole("button", { name: "Anlegen" }));
    expect(await screen.findByText("Probe: lesen, beteiligt gespeichert.")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Probe: lesen" })).toBeDisabled();
    expect(screen.getByText("eingeschränkt")).toBeInTheDocument();

    const puts = calls(state).filter((c) => c.call.startsWith("PUT"));
    expect(puts.map((c) => c.body)).toEqual([
      { type_name: "probe.probe", action: "read", scope: "all", condition: "record.priority >" },
      {
        type_name: "probe.probe",
        action: "read",
        scope: "all",
        condition: "record.priority > 1",
        fields: { read: ["title", "priority", "active", "seen_at", "kind", "parent"] },
      },
      { type_name: "probe.probe", action: "read", scope: "involved" },
    ]);
  });

  it("grants and withdraws a special right, and assigns the role to a person at a node", async () => {
    const user = userEvent.setup();
    const view = renderAt("/roles/agent");
    await screen.findByRole("heading", { name: "Rolle: Bearbeiter" });
    const customize = screen.getByRole("checkbox", { name: "Typen und Workflows anpassen" });
    await user.click(customize);
    expect(await screen.findByText("„Typen und Workflows anpassen“ erteilt.")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Typen und Workflows anpassen" })).toBeChecked();
    await user.click(screen.getByRole("checkbox", { name: "Typen und Workflows anpassen" }));
    expect(await screen.findByText("„Typen und Workflows anpassen“ entzogen.")).toBeInTheDocument();

    const form = screen.getByRole("form", { name: "Neue Zuweisung" });
    await user.type(within(form).getByRole("combobox", { name: "Person oder Gruppe" }), "ber");
    await user.click(await screen.findByRole("option", { name: /^Berg/ }));
    await selectOption(user, within(form).getByLabelText("Knoten"), "root.it");
    await user.click(within(form).getByRole("button", { name: "Zuweisen" }));
    expect(await screen.findByText("Rolle an IT (root.it) zugewiesen.")).toBeInTheDocument();
    const table = screen.getByRole("region", { name: "Zuweisungen" });
    expect(await within(table).findByText("Anna Berg")).toBeInTheDocument();
    await expectAccessible(view.container);
    await user.click(
      within(table).getByRole("button", { name: "Zuweisung an IT (root.it) entfernen" }),
    );
    expect(await screen.findByText("Niemand hat diese Rolle.")).toBeInTheDocument();

    expect(calls(state)).toEqual([
      {
        call: "PUT /api/v1/roles/agent/permissions",
        body: { type_name: "core.type", action: "customize", scope: "all" },
      },
      { call: "DELETE /api/v1/roles/agent/permissions/core.type/customize", body: undefined },
      {
        call: "POST /api/v1/assignments",
        body: { role: "agent", person_id: anna, org_path: "root.it" },
      },
      { call: "DELETE /api/v1/assignments/assign-1", body: undefined },
    ]);
  });
});
