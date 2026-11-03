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

  it("sets a global default at a node, and says when the right is missing", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/settings/global");
    await screen.findByRole("heading", { name: "Globale Einstellungen" });
    await screen.findByRole("option", { name: "IT (root.it)" });
    await user.selectOptions(screen.getByLabelText("Knoten"), "root.it");
    const zone = screen.getByRole("listitem", { name: "Zeitzone" });
    await user.selectOptions(within(zone).getByLabelText("Zeitzone"), "Europe/London");
    await user.click(within(zone).getByRole("button", { name: "Speichern" }));
    expect(await within(zone).findByText("Gilt hier aus root.it")).toBeInTheDocument();
    expect(state.orgSettings?.["root.it"]).toEqual({ "ui.time_zone": "Europe/London" });
    await expectAccessible(container);

    await user.click(within(zone).getByRole("button", { name: "Eigenen Wert entfernen" }));
    expect(
      await within(zone).findByText("Nicht eingestellt: es gilt die Vorgabe."),
    ).toBeInTheDocument();

    state.orgForbidden = true;
    await user.selectOptions(within(zone).getByLabelText("Zeitzone"), "Europe/Paris");
    await user.click(within(zone).getByRole("button", { name: "Speichern" }));
    expect(await screen.findByText(/fehlt dir das Recht manage_settings/)).toBeInTheDocument();
  });
});
