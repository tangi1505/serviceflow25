// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Settings (UI9, UI38): English on "Meine Einstellungen" makes the interface English at once,
// also when it starts again (signed in anew); in the tab Darstellung dark and the accent apply
// at once and go back with "Auf Vorgabe zurücksetzen"; presets of a type are kept a moment
// after the change and fill the form of a new record; the global page sets a default at a node,
// and says when the right is missing.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
    expect(screen.getByRole("tab", { name: "Sprache und Zeit" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByText("Gilt jetzt: de (Vorgabe)")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Speichern" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Theme")).not.toBeInTheDocument();
    await expectAccessible(first.container);
    // Every change applies at once (E49).
    await selectOption(user, screen.getByLabelText("Sprache"), "en");
    expect(await screen.findByRole("heading", { name: "My settings" })).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("en");
    await selectOption(user, screen.getByLabelText("Time zone"), "Europe/London");
    await waitFor(() =>
      expect(state.settings).toMatchObject({ language: "en", time_zone: "Europe/London" }),
    );
    first.unmount();

    // A new start (the browser says German): the person's language wins.
    initI18n("de");
    renderAt("/");
    expect(await screen.findByRole("navigation", { name: "Types" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Account of Pascal" })).toBeInTheDocument();
  });

  it("chooses dark and indigo, at once and on the next start, and goes back to the default", async () => {
    const user = userEvent.setup();
    const first = renderAt("/settings");
    await screen.findByRole("heading", { name: "Meine Einstellungen" });
    await user.click(screen.getByRole("tab", { name: "Darstellung" }));
    expect(screen.getByText("Gilt jetzt: Wie das System (Vorgabe)")).toBeInTheDocument();
    await expectAccessible(first.container);
    await user.click(screen.getByRole("button", { name: "Dunkel" }));
    expect(await screen.findByText("Gespeichert.")).toBeInTheDocument();
    expect(state.settings).toMatchObject({ color_scheme: "dark" });
    expect(screen.getByRole("button", { name: "Dunkel" })).toHaveAttribute("aria-pressed", "true");
    expect(document.documentElement.dataset.colorScheme).toBe("dark");
    // Indigo: light, dark and the soft accent derived (§2.3).
    await user.click(screen.getByRole("button", { name: "Indigo" }));
    await waitFor(() =>
      expect(document.documentElement.style.getPropertyValue("--pick-accent-light")).toBe(
        "#3F4BB8",
      ),
    );
    expect(document.documentElement.style.getPropertyValue("--pick-accent-dark")).toBe("#9097d6");
    expect(state.settings).toMatchObject({ accent: "indigo" });
    await user.click(screen.getByRole("button", { name: "Kompakt" }));
    await waitFor(() => expect(state.settings).toMatchObject({ density: "compact" }));
    first.unmount();
    // A new start begins light (the browser of the test), the person's setting turns it dark.
    document.documentElement.dataset.colorScheme = "light";
    renderAt("/");
    await screen.findByRole("navigation", { name: "Typen" });
    await waitFor(() => expect(document.documentElement.dataset.colorScheme).toBe("dark"));
    expect(document.documentElement.style.getPropertyValue("--pick-accent-light")).toBe("#3F4BB8");

    // Back to the default: the unit's values or the core's, petrol again.
    await user.click(await screen.findByRole("button", { name: /^Konto von / }));
    await user.click(screen.getByRole("link", { name: "Meine Einstellungen" }));
    await user.click(await screen.findByRole("tab", { name: "Darstellung" }));
    await user.click(screen.getByRole("button", { name: "Auf Vorgabe zurücksetzen" }));
    await waitFor(() =>
      expect(document.documentElement.style.getPropertyValue("--pick-accent-light")).toBe(
        "#0B6E80",
      ),
    );
    expect(state.settings?.["accent"]).toBeUndefined();
    expect(state.settings?.["color_scheme"]).toBeUndefined();
  });

  it("fills a new record with the presets of its type", async () => {
    const user = userEvent.setup();
    const first = renderAt("/settings");
    await screen.findByRole("heading", { name: "Meine Einstellungen" });
    await user.click(screen.getByRole("tab", { name: "Vorbelegung" }));
    const presets = screen.getByRole("region", { name: "Vorbelegung beim Anlegen" });
    await selectOption(user, within(presets).getByLabelText("Typ"), "probe.probe");
    await user.click(within(presets).getByRole("button", { name: "Feld hinzufügen" }));
    const row = within(presets).getByRole("listitem", { name: "Vorbelegung 1" });
    await selectOption(user, within(row).getByRole("combobox", { name: "Feld" }), "kind");
    // The value is the select field of the kit (UI57, UI61).
    await user.click(within(row).getByRole("combobox", { name: "Neuer Wert" }));
    await user.click(await screen.findByRole("option", { name: "Störung" }));
    // Kept a moment after the last change, without a button (E49).
    expect(
      // As a message at the bottom right, outside the section (UI60).
      await screen.findByText("Gespeichert.", {}, { timeout: 3000 }),
    ).toBeInTheDocument();
    expect(state.settings?.["defaults"]).toEqual({ "probe.probe": { kind: "fault" } });
    first.unmount();

    renderAt("/types/probe.probe/new");
    await screen.findByRole("heading", { name: "Neuer Datensatz" });
    expect(screen.getByLabelText("Art")).toHaveAttribute("data-value", "fault");
  });

  // SD20: the page shows the declared settings by group and saves all changes at once in the
  // bar at the top (Pascal's comment on the mockup); before SD20 every setting had its own
  // button, so this test replaces the one of UI9 that saved the time zone by itself.
  it("saves all changes of a unit at once in the bar at the top", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/settings/global");
    await screen.findByRole("heading", { name: "Globale Einstellungen · Gesamt" });
    await user.click(screen.getByLabelText("Einheit"));
    await screen.findByRole("option", { name: "IT (root.it)" });
    await selectOption(user, screen.getByLabelText("Einheit"), "root.it");
    await screen.findByRole("heading", { name: "Globale Einstellungen · IT" });
    expect(screen.getByRole("heading", { name: "Oberfläche Kern" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Störung Paket incident" })).toBeInTheDocument();
    const bar = screen.getByRole("region", { name: "Speichern" });
    expect(within(bar).getByRole("status")).toHaveTextContent("Alles gespeichert");
    expect(within(bar).getByRole("button", { name: "Speichern" })).toBeDisabled();

    const zone = screen.getByRole("group", { name: "Zeitzone" });
    expect(within(zone).getByText("Vorgabe des Kerns")).toBeInTheDocument();
    await selectOption(user, within(zone).getByLabelText("Zeitzone"), "Europe/London");
    expect(within(zone).getByText("Geändert, noch nicht gespeichert")).toBeInTheDocument();
    const matrix = screen.getByRole("group", { name: "Prioritätsmatrix" });
    await selectOption(
      user,
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
    await user.click(await screen.findByLabelText("Einheit"));
    await screen.findByRole("option", { name: "IT (root.it)" });
    await selectOption(user, screen.getByLabelText("Einheit"), "root.it");
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
