// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The appearance of the organisation (UI39, UI plan E51): a new name shows in the header and the
// title of the browser after "Speichern"; a colour below 4.5 : 1 on white is an error at the
// field and nothing is saved; what the organisation switches off is gone from Meine
// Einstellungen; only a session with the right sees the page in the navigation.
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

describe("Erscheinungsbild", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    localStorage.clear();
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    document.documentElement.removeAttribute("style");
  });

  it("names the application, checks the colour and saves only on Speichern", async () => {
    state.appearance = true;
    const user = userEvent.setup();
    const view = renderAt("/");
    const nav = await screen.findByRole("navigation", { name: "Typen" });
    await user.click(await within(nav).findByRole("link", { name: "Erscheinungsbild" }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "Erscheinungsbild" }),
    ).toBeInTheDocument();
    await expectAccessible(view.container);
    // Without a name and a logo of its own: the mark "sf" and ServiceFlow25 (UI56).
    const banner = screen.getByRole("banner");
    expect(within(banner).getByText("sf")).toBeInTheDocument();
    expect(within(banner).getByText("ServiceFlow25")).toBeInTheDocument();

    // Beside the defaults a small picture follows the choices at once (UI55).
    const preview = screen.getByRole("img", {
      name: "Vorschau: Wie das System, Zeilenhöhe Normal, Leiste Breit",
    });
    await user.selectOptions(screen.getByRole("combobox", { name: "Modus" }), "dark");
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Zeilenhöhe der Listen" }),
      "compact",
    );
    await user.selectOptions(screen.getByRole("combobox", { name: "Breite der Leiste" }), "narrow");
    expect(preview).toHaveAccessibleName("Vorschau: Dunkel, Zeilenhöhe Kompakt, Leiste Schmal");
    expect(preview).toHaveAttribute("data-color-scheme", "dark");

    const name = screen.getByLabelText("Name der Anwendung");
    await user.clear(name);
    await user.type(name, "Leitstelle Nord");
    // Nothing is kept before Speichern.
    expect(state.orgSettings?.["root"]?.["ui.app_name"]).toBeUndefined();

    // 4.48 : 1 is too weak: an error at the field, and no saving.
    const color = screen.getByRole("textbox", { name: "Akzentfarbe" });
    await user.clear(color);
    await user.type(color, "#777777");
    expect(screen.getByText(/zu schwach, mindestens 4,5 : 1/)).toBeInTheDocument();
    expect(color).toHaveAttribute("aria-invalid", "true");
    const save = screen.getByRole("button", { name: "Speichern" });
    expect(save).toBeDisabled();

    await user.clear(color);
    await user.type(color, "#1F5FA8");
    expect(screen.getByText(/erfüllt AA/)).toBeInTheDocument();
    await user.click(screen.getByRole("checkbox", { name: /Hauptaktion in der Akzentfarbe/ }));
    await user.click(save);
    await waitFor(() =>
      expect(state.orgSettings?.["root"]).toMatchObject({
        "ui.app_name": "Leitstelle Nord",
        "ui.accent": "#1F5FA8",
        "ui.primary_accent": true,
      }),
    );
    expect(await screen.findByText("Gespeichert. Gilt ab jetzt für alle.")).toBeInTheDocument();
    // The header and the title of the browser show the name; the main action takes the accent.
    const header = screen.getByRole("banner");
    expect(await within(header).findByText("Leitstelle Nord")).toBeInTheDocument();
    await waitFor(() => expect(document.title).toBe("Leitstelle Nord"));
    expect(document.documentElement.style.getPropertyValue("--pick-primary-light")).toBe("#1F5FA8");
  });

  it("takes a choice away from Meine Einstellungen when the organisation switches it off", async () => {
    state.orgSettings = { root: { "ui.own_accent": false, "ui.own_nav": false } };
    const user = userEvent.setup();
    const view = renderAt("/settings");
    await user.click(await screen.findByRole("tab", { name: "Darstellung" }));
    expect(screen.getByRole("button", { name: "Dunkel" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Indigo" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Navigation" })).not.toBeInTheDocument();
    expect(screen.getByText("Stellt deine Organisation ein.")).toBeInTheDocument();
    // Without arranging, the bar has no stars and no collapsing.
    const nav = screen.getByRole("navigation", { name: "Typen" });
    expect(
      within(nav).queryByRole("button", { name: /Unter Favoriten anheften/ }),
    ).not.toBeInTheDocument();
    expect(
      within(nav).queryByRole("button", { name: "Leiste einklappen" }),
    ).not.toBeInTheDocument();
    // Without the right there is no page Erscheinungsbild.
    expect(within(nav).queryByRole("link", { name: "Erscheinungsbild" })).not.toBeInTheDocument();
    await expectAccessible(view.container);
  });
});
