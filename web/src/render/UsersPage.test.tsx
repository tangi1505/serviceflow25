// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Users (UI66): the table with origin, state and roles; a new user with an invitation and one
// with a one-time password; at a user a role added and removed after the question, blocked
// after the question; the page of an invitation sets the password once.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { answer } from "@/test/confirm";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";
import { selectOption } from "@/test/select";
import { expectAccessible } from "@/test/setup";

function renderAt(path: string) {
  const queryClient = createQueryClient();
  const router = createAppRouter({ queryClient, history: memoryHistory(path) });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

const loaded = { timeout: 5000 };

describe("UsersPage", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.manageRights = true;
    state.roles = [
      { name: "servicedesk", label: { de: "Servicedesk" }, permissions: [] },
      { name: "leser", label: { de: "Leser" }, permissions: [] },
    ] as unknown as FakeState["roles"];
    state.accounts = [
      {
        id: "acc-anna",
        username: "a_berg",
        display_name: "Anna Berg",
        origin: "local",
        person_id: "p-anna",
        active: true,
        password_expired: false,
        last_login_at: "2026-11-19T08:00:00Z",
        created_at: "2026-11-01T10:00:00Z",
      },
      {
        id: "acc-ben",
        username: "b_kc",
        display_name: "Ben Keycloak",
        origin: "oidc",
        issuer: "keycloak",
        person_id: "p-ben",
        active: false,
        password_expired: false,
        created_at: "2026-11-02T10:00:00Z",
      },
    ];
    state.assignments = [
      { id: "as-1", role: "servicedesk", person_id: "p-anna", org_path: "root.it" },
    ];
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists users with origin, state and roles, and filters them", async () => {
    const user = userEvent.setup();
    renderAt("/users");
    const table = await screen.findByRole("table", {}, loaded);
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Anna Berg");
    expect(rows[0]).toHaveTextContent("a_berg");
    expect(rows[0]).toHaveTextContent("lokal");
    expect(rows[0]).toHaveTextContent("Servicedesk (IT)");
    expect(rows[1]).toHaveTextContent("Identitätsanbieter (keycloak)");
    expect(rows[1]).toHaveTextContent("gesperrt");
    await expectAccessible(document.body);
    await selectOption(user, screen.getByRole("combobox", { name: "Rolle" }), "servicedesk");
    expect(within(table).getAllByRole("row")).toHaveLength(2);
    await user.type(screen.getByLabelText("Name oder Benutzername"), "keiner");
    expect(await screen.findByText("Keine Benutzer gefunden.")).toBeInTheDocument();
  });

  it("creates a user with an invitation and one with a one-time password", async () => {
    const user = userEvent.setup();
    const router = renderAt("/users");
    await user.click(await screen.findByRole("button", { name: "Neuer Benutzer" }, loaded));
    await screen.findByRole("heading", { name: "Neuer Benutzer" });
    await user.type(screen.getByLabelText("Vorname"), "Carla");
    await user.type(screen.getByLabelText(/^Nachname/), "Neu");
    await user.type(screen.getByLabelText(/^Benutzername/), "c_neu");
    await user.type(screen.getByLabelText(/^E-Mail/), "carla@example.test");
    await expectAccessible(document.body);
    await user.click(screen.getByRole("button", { name: "Anlegen und einladen" }));
    expect(
      await screen.findByText("Angelegt, Einladung an carla@example.test gesendet"),
    ).toBeInTheDocument();
    await waitFor(() => expect(router.state.location.pathname).toBe("/users/acc-3"));
    expect(state.accounts!.find((a) => a.username === "c_neu")?.invited_email).toBe(
      "carla@example.test",
    );

    void router.navigate({ to: "/users/new" });
    await screen.findByRole("heading", { name: "Neuer Benutzer" });
    await user.type(screen.getByLabelText(/^Nachname/), "Einmal");
    await user.type(screen.getByLabelText(/^Benutzername/), "a_berg");
    await user.click(screen.getByRole("radio", { name: /Einmal-Kennwort/ }));
    await user.type(screen.getByRole("textbox", { name: /^Einmal-Kennwort/ }), "Einmal-Pw7!x");
    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    // The name is taken: the field says so.
    expect(await screen.findByText("Diesen Benutzernamen gibt es schon.")).toBeInTheDocument();
    await user.clear(screen.getByLabelText(/^Benutzername/));
    await user.type(screen.getByLabelText(/^Benutzername/), "e_einmal");
    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    expect(
      await screen.findByText("Angelegt; beim ersten Anmelden ändert die Person das Kennwort"),
    ).toBeInTheDocument();
  });

  it("says without a mail server that an invitation cannot go out", async () => {
    const user = userEvent.setup();
    state.noMail = true;
    renderAt("/users/new");
    await screen.findByRole("heading", { name: "Neuer Benutzer" }, loaded);
    await user.type(screen.getByLabelText(/^Nachname/), "Ohne");
    await user.type(screen.getByLabelText(/^Benutzername/), "o_mail");
    await user.type(screen.getByLabelText(/^E-Mail/), "ohne@example.test");
    await user.click(screen.getByRole("button", { name: "Anlegen und einladen" }));
    expect(await screen.findByText(/kein Mailserver eingerichtet/)).toBeInTheDocument();
    expect(state.accounts!.some((a) => a.username === "o_mail")).toBe(false);
  });

  it("says why a new invitation cannot go out", async () => {
    const user = userEvent.setup();
    renderAt("/users/acc-anna");
    await screen.findByRole("heading", { name: "Anna Berg" }, loaded);
    const again = async () => {
      await user.click(screen.getByRole("button", { name: "Aktionen" }));
      await user.click(screen.getByRole("menuitem", { name: "Neu einladen" }));
      await screen.findByRole("alertdialog");
      await user.keyboard("{Enter}");
    };
    await again();
    expect(await screen.findByText(/keine E-Mail-Adresse/)).toBeInTheDocument();
    state.noMail = true;
    await again();
    expect(await screen.findByText(/kein Mailserver eingerichtet/)).toBeInTheDocument();
  });

  it("adds and removes a role at a user and blocks after the question", async () => {
    const user = userEvent.setup();
    renderAt("/users/acc-anna");
    await screen.findByRole("heading", { name: "Anna Berg" }, loaded);
    const roles = screen.getByRole("region", { name: "Rollen" });
    expect(within(roles).getByRole("table")).toHaveTextContent("Servicedesk");
    await selectOption(user, within(roles).getByRole("combobox", { name: "Rolle" }), "leser");
    await user.click(within(roles).getByRole("button", { name: "Rolle hinzufügen" }));
    expect(await screen.findByText("Rolle hinzugefügt")).toBeInTheDocument();
    expect(state.assignments!.some((a) => a.role === "leser" && a.person_id === "p-anna")).toBe(
      true,
    );
    await expectAccessible(document.body);

    await user.click(
      within(roles).getByRole("button", { name: "Rolle Servicedesk an IT entfernen" }),
    );
    await answer(user, "Entfernen");
    await waitFor(() =>
      expect(state.assignments!.some((a) => a.role === "servicedesk")).toBe(false),
    );

    await user.click(screen.getByRole("button", { name: "Aktionen" }));
    await user.click(screen.getByRole("menuitem", { name: "Sperren" }));
    await screen.findByRole("alertdialog", { name: "„Anna Berg“ sperren?" });
    await user.keyboard("{Enter}");
    await waitFor(() => expect(state.accounts![0]!.active).toBe(false));
  });

  it("sets the password with the key of an invitation, once", async () => {
    const user = userEvent.setup();
    state.signedIn = false;
    state.invitations = { k1: "c_neu" };
    renderAt("/invite?key=k1");
    await screen.findByRole("heading", { name: "Kennwort setzen" }, loaded);
    await user.type(screen.getByLabelText("Neues Kennwort"), "Gesetzt-Pw9!z");
    await user.type(screen.getByLabelText("Kennwort wiederholen"), "anders");
    await user.click(screen.getByRole("button", { name: "Kennwort setzen" }));
    expect(await screen.findByText("Die beiden Kennwörter sind nicht gleich.")).toBeInTheDocument();
    await user.clear(screen.getByLabelText("Kennwort wiederholen"));
    await user.type(screen.getByLabelText("Kennwort wiederholen"), "Gesetzt-Pw9!z");
    await user.click(screen.getByRole("button", { name: "Kennwort setzen" }));
    expect(await screen.findByText(/Melden Sie sich als „c_neu“ an/)).toBeInTheDocument();
    expect(state.invitations).toEqual({});
    await expectAccessible(document.body);
  });
});
