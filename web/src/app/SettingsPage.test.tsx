// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Settings (UI9): English on "Meine Einstellungen" makes the interface English, also when it
// starts again (signed in anew); presets of a type fill the form of a new record; the global
// page sets a default at a node, and says when the right is missing.
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

describe("Settings", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("switches the interface to English, also after signing in again", async () => {
    const user = userEvent.setup();
    const first = renderAt("/settings");
    await screen.findByRole("heading", { name: "Meine Einstellungen" });
    expect(screen.getByText("Gilt jetzt: de (Vorgabe)")).toBeInTheDocument();
    await expectAccessible(first.container);
    await user.selectOptions(
      screen.getByLabelText("Sprache", { selector: "select:not([class*='h-9'])" }),
      "en",
    );
    await user.selectOptions(screen.getByLabelText("Zeitzone"), "Europe/London");
    await user.click(screen.getByRole("button", { name: "Speichern" }));
    expect(await screen.findByRole("heading", { name: "My settings" })).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("en");
    expect(state.settings).toMatchObject({ language: "en", time_zone: "Europe/London" });
    first.unmount();

    // A new start (the browser says German): the person's language wins.
    initI18n("de");
    renderAt("/");
    expect(await screen.findByRole("navigation", { name: "Types" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign out" })).toBeInTheDocument();
  });

  it("chooses dark, and the page turns dark at once and on the next start", async () => {
    const user = userEvent.setup();
    const first = renderAt("/settings");
    await screen.findByRole("heading", { name: "Meine Einstellungen" });
    const scheme = screen.getByLabelText("Darstellung");
    expect(scheme).toHaveValue("");
    expect(screen.getByText("Gilt jetzt: system (Vorgabe)")).toBeInTheDocument();
    await user.selectOptions(scheme, "dark");
    await user.click(screen.getByRole("button", { name: "Speichern" }));
    expect(await screen.findByText("Gespeichert.")).toBeInTheDocument();
    expect(state.settings).toMatchObject({ color_scheme: "dark" });
    expect(document.documentElement.dataset.colorScheme).toBe("dark");
    first.unmount();
    // A new start begins light (the browser of the test), the person's setting turns it dark.
    document.documentElement.dataset.colorScheme = "light";
    renderAt("/");
    await screen.findByRole("navigation", { name: "Typen" });
    await waitFor(() => expect(document.documentElement.dataset.colorScheme).toBe("dark"));
  });

  it("fills a new record with the presets of its type", async () => {
    const user = userEvent.setup();
    const first = renderAt("/settings");
    await screen.findByRole("heading", { name: "Meine Einstellungen" });
    const presets = screen.getByRole("region", { name: "Vorbelegung beim Anlegen" });
    await user.selectOptions(within(presets).getByLabelText("Typ"), "probe.probe");
    await user.click(within(presets).getByRole("button", { name: "Feld hinzufügen" }));
    const row = within(presets).getByRole("listitem", { name: "Vorbelegung 1" });
    await user.selectOptions(within(row).getByRole("combobox", { name: "Feld" }), "kind");
    await user.selectOptions(within(row).getByRole("combobox", { name: "Neuer Wert" }), "fault");
    await user.click(within(presets).getByRole("button", { name: "Vorbelegung speichern" }));
    expect(await within(presets).findByText("Gespeichert.")).toBeInTheDocument();
    expect(state.settings?.["defaults"]).toEqual({ "probe.probe": { kind: "fault" } });
    first.unmount();

    renderAt("/types/probe.probe/new");
    await screen.findByRole("heading", { name: "Neuer Datensatz" });
    expect(screen.getByLabelText("Art")).toHaveValue("fault");
  });

  // SD20: the page shows the declared settings by group and saves all changes at once in the
  // bar at the top (Pascal's comment on the mockup); before SD20 every setting had its own
  // button, so this test replaces the one of UI9 that saved the time zone by itself.
  it("saves all changes of a unit at once in the bar at the top", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/settings/global");
    await screen.findByRole("heading", { name: "Globale Einstellungen · Gesamt" });
    await screen.findByRole("option", { name: "IT (root.it)" });
    await user.selectOptions(screen.getByLabelText("Einheit"), "root.it");
    await screen.findByRole("heading", { name: "Globale Einstellungen · IT" });
    expect(screen.getByRole("heading", { name: "Oberfläche Kern" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Störung Paket incident" })).toBeInTheDocument();
    const bar = screen.getByRole("region", { name: "Speichern" });
    expect(within(bar).getByRole("status")).toHaveTextContent("Alles gespeichert");
    expect(within(bar).getByRole("button", { name: "Speichern" })).toBeDisabled();

    const zone = screen.getByRole("group", { name: "Zeitzone" });
    expect(within(zone).getByText("Vorgabe des Kerns")).toBeInTheDocument();
    await user.selectOptions(within(zone).getByLabelText("Zeitzone"), "Europe/London");
    expect(within(zone).getByText("Geändert, noch nicht gespeichert")).toBeInTheDocument();
    const matrix = screen.getByRole("group", { name: "Prioritätsmatrix" });
    await user.selectOptions(
      within(matrix).getByLabelText("Auswirkung Hoch, Dringlichkeit Niedrig"),
      "1",
    );
    await user.clear(
      screen.getByRole("spinbutton", { name: "Gelöste Störung automatisch schließen nach" }),
    );
    await user.type(
      screen.getByRole("spinbutton", { name: "Gelöste Störung automatisch schließen nach" }),
      "36",
    );
    const allowed = screen.getByRole("group", { name: "Erlaubte Endungen" });
    expect(within(allowed).getByText("Keine Einträge")).toBeInTheDocument();
    await user.type(
      within(allowed).getByLabelText("Eintrag zu „Erlaubte Endungen“ hinzufügen"),
      "pdf{Enter}",
    );
    expect(within(allowed).getByRole("button", { name: "pdf entfernen" })).toBeInTheDocument();
    // A setting of the root only cannot be changed at a unit below.
    const login = screen.getByRole("group", { name: "Lokale Anmeldung" });
    expect(within(login).getByLabelText("Lokale Anmeldung")).toBeDisabled();
    expect(within(login).getByText("Nur am Wurzelknoten einstellbar.")).toBeInTheDocument();
    expect(within(bar).getByRole("status")).toHaveTextContent("4 Änderungen nicht gespeichert");
    await expectAccessible(container);

    await user.click(within(bar).getByRole("button", { name: "Speichern" }));
    expect(
      await screen.findByText("Gespeichert: 4 Einstellungen für IT und alle Einheiten darunter."),
    ).toBeInTheDocument();
    expect(state.orgSettings?.["root.it"]).toEqual({
      "ui.time_zone": "Europe/London",
      "incident.priority_matrix": { high: { high: 1, low: 1 }, low: { high: 2, low: 3 } },
      "incident.close_after": 2160,
      "files.allowed_extensions": ["pdf"],
    });
    expect(await within(zone).findByText("Eigene Einstellung dieser Einheit")).toBeInTheDocument();
    expect(within(bar).getByRole("status")).toHaveTextContent("Alles gespeichert");
  });

  it("removes an own value, shows what the server refuses, and says when the right is missing", async () => {
    state.orgSettings = {
      root: { "files.max_size": 20971520 },
      "root.it": { "ui.time_zone": "Europe/London" },
    };
    const user = userEvent.setup();
    renderAt("/settings/global");
    await screen.findByRole("option", { name: "IT (root.it)" });
    await user.selectOptions(screen.getByLabelText("Einheit"), "root.it");
    const zone = await screen.findByRole("group", { name: "Zeitzone" });
    expect(await within(zone).findByText("Eigene Einstellung dieser Einheit")).toBeInTheDocument();
    const size = screen.getByRole("group", { name: "Höchstgröße einer Datei" });
    expect(within(size).getByText("Geerbt von Gesamt")).toBeInTheDocument();
    expect(within(size).getByLabelText("Höchstgröße einer Datei")).toHaveValue(20);
    const bar = screen.getByRole("region", { name: "Speichern" });

    // Remove the own value, then discard it.
    await user.click(within(zone).getByRole("button", { name: "Eigene entfernen" }));
    expect(within(zone).getByText(/Wird beim Speichern entfernt/)).toBeInTheDocument();
    await user.click(within(bar).getByRole("button", { name: "Verwerfen" }));
    expect(within(zone).getByText("Eigene Einstellung dieser Einheit")).toBeInTheDocument();

    // The server refuses a value: nothing is saved, the message stands at the setting.
    state.orgInvalid = { "files.max_size": "must be at least 1" };
    await user.clear(within(size).getByLabelText("Höchstgröße einer Datei"));
    await user.type(within(size).getByLabelText("Höchstgröße einer Datei"), "0");
    await user.click(within(zone).getByRole("button", { name: "Eigene entfernen" }));
    await user.click(within(bar).getByRole("button", { name: "Speichern" }));
    expect(
      await screen.findByText("Nicht gespeichert: Bitte die markierten Einstellungen prüfen."),
    ).toBeInTheDocument();
    expect(within(size).getByText("must be at least 1")).toBeInTheDocument();
    expect(state.orgSettings["root.it"]).toEqual({ "ui.time_zone": "Europe/London" });
    expect(within(bar).getByRole("status")).toHaveTextContent("2 Änderungen nicht gespeichert");

    state.orgInvalid = undefined;
    state.orgForbidden = true;
    await user.click(within(bar).getByRole("button", { name: "Speichern" }));
    expect(await screen.findByText(/fehlt dir das Recht manage_settings/)).toBeInTheDocument();
  });
});
