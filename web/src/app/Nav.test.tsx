// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The navigation (UI36, UI plan E41, E48): areas named and drawn by their package, to open and
// close, closed with the number of their entries, kept per account in the browser; the narrow
// bar with symbols shows the entries of an area beside it and goes back to wide. Favourites and
// the own arrangement (UI37, UI plan E50): a star pins an entry under Favoriten; Meine
// Einstellungen orders and hides the areas and orders the favourites, kept on the server.
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

describe("Navigation", () => {
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
  });

  it("opens and closes the areas of the packages and keeps them closed", async () => {
    const user = userEvent.setup();
    const view = renderAt("/");
    const nav = await screen.findByRole("navigation", { name: "Typen" });
    // The area is named by its package (E48), not by a raw name.
    const area = await within(nav).findByRole("button", { name: "Probe" });
    expect(area).toHaveAttribute("aria-expanded", "true");
    expect(within(nav).getByRole("link", { name: "Probe" })).toHaveAttribute(
      "href",
      "/types/probe.probe",
    );
    expect(within(nav).getByRole("link", { name: "Start" })).toHaveAttribute("href", "/");
    expect(within(nav).getByRole("button", { name: "Dashboards" })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: "Neues Dashboard" })).toBeInTheDocument();
    await expectAccessible(view.container);

    await user.click(area);
    expect(area).toHaveAttribute("aria-expanded", "false");
    expect(area).toHaveTextContent("Probe1");
    expect(within(nav).queryByRole("link", { name: "Probe" })).not.toBeInTheDocument();
    view.unmount();

    // Again, the area stays closed for this account.
    renderAt("/");
    const again = await screen.findByRole("button", { name: "Probe" });
    await waitFor(() => expect(again).toHaveAttribute("aria-expanded", "false"));
  });

  it("narrows the bar to symbols, shows the entries of an area beside it and widens it again", async () => {
    const user = userEvent.setup();
    const view = renderAt("/");
    await user.click(await screen.findByRole("button", { name: "Leiste einklappen" }));
    await waitFor(() =>
      expect(
        state.calls.some(
          (c) =>
            c.path === "/api/v1/me/settings" &&
            c.method === "PATCH" &&
            c.body === JSON.stringify({ nav: "narrow" }),
        ),
      ).toBe(true),
    );
    // The narrow bar is the one navigation now, of symbols only.
    const symbol = await screen.findByRole("button", { name: "Probe", expanded: false });
    const rail = screen.getByRole("navigation", { name: "Typen" });
    expect(within(rail).queryByRole("link", { name: "Probe" })).not.toBeInTheDocument();
    await user.click(symbol);
    const beside = within(rail).getByRole("group", { name: "Probe" });
    expect(within(beside).getByRole("link", { name: "Probe" })).toHaveAttribute(
      "href",
      "/types/probe.probe",
    );
    await expectAccessible(view.container);
    await user.keyboard("{Escape}");
    expect(beside).not.toBeInTheDocument();
    expect(symbol).toHaveFocus();

    await user.click(symbol);
    await user.click(
      within(within(rail).getByRole("group", { name: "Probe" })).getByRole("link", {
        name: "Probe",
      }),
    );
    await waitFor(() => expect(view.router.state.location.pathname).toBe("/types/probe.probe"));
    expect(within(rail).queryByRole("group", { name: "Probe" })).not.toBeInTheDocument();

    await user.click(within(rail).getByRole("button", { name: "Leiste ausklappen" }));
    expect(await screen.findByRole("button", { name: "Leiste einklappen" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Probe" })).toHaveAttribute(
      "href",
      "/types/probe.probe",
    );
  });

  it("pins an entry with its star under Favoriten and unpins it", async () => {
    const user = userEvent.setup();
    const view = renderAt("/");
    const nav = await screen.findByRole("navigation", { name: "Typen" });
    await user.click(
      await within(nav).findByRole("button", { name: "Unter Favoriten anheften: Probe" }),
    );
    // Favoriten stands on top with the entry, at once and on the server.
    const fav = await within(nav).findByRole("button", { name: "Favoriten" });
    const heads = within(nav)
      .getAllByRole("button", { expanded: true })
      .map((b) => b.textContent);
    expect(heads[0]).toBe("Favoriten");
    expect(within(nav).getAllByRole("link", { name: "Probe" })).toHaveLength(2);
    const stars = within(nav).getAllByRole("button", { name: "Von den Favoriten lösen: Probe" });
    expect(stars).toHaveLength(2);
    expect(stars[0]).toHaveAttribute("aria-pressed", "true");
    await waitFor(() =>
      expect(state.settings!["favorites"]).toEqual(["package.probe/probe.probe"]),
    );
    await expectAccessible(view.container);

    await user.click(stars[0]!);
    await waitFor(() => expect(fav).not.toBeInTheDocument());
    expect(within(nav).getAllByRole("link", { name: "Probe" })).toHaveLength(1);
    expect(state.settings!["favorites"]).toEqual([]);
  });

  it("orders and hides the areas and keeps the favourites in Meine Einstellungen", async () => {
    // A favourite that leads nowhere now (a dashboard gone) is not shown but kept.
    state.settings = { favorites: ["dashboards/gone", "package.probe/probe.probe"] };
    const user = userEvent.setup();
    const view = renderAt("/settings");
    await user.click(await screen.findByRole("tab", { name: "Darstellung" }));
    const nav = screen.getByRole("navigation", { name: "Typen" });
    const areaNames = () =>
      within(nav)
        .getAllByRole("button", { expanded: true })
        .map((b) => b.textContent);
    const before = (a: string, b: string) => areaNames().indexOf(a) < areaNames().indexOf(b);
    await waitFor(() => expect(areaNames()[0]).toBe("Favoriten"));
    expect(before("Dashboards", "Probe")).toBe(true);
    expect(screen.getByRole("button", { name: "Dashboards nach oben" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "Probe nach oben" }));
    await waitFor(() => expect(before("Probe", "Dashboards")).toBe(true));
    expect((state.settings!["nav_order"] as string[]).slice(0, 2)).toEqual([
      "package.probe",
      "dashboards",
    ]);

    await user.click(screen.getByRole("checkbox", { name: "Probe anzeigen" }));
    await waitFor(() => expect(areaNames()).not.toContain("Probe"));
    expect(state.settings!["nav_hidden"]).toEqual(["package.probe"]);
    // The favourite of the hidden area stays under Favoriten.
    expect(within(nav).getByRole("link", { name: "Probe" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Favorit Probe nach oben" })).toBeDisabled();
    await expectAccessible(view.container);

    await user.click(screen.getByRole("button", { name: "Favorit Probe lösen" }));
    await waitFor(() => expect(state.settings!["favorites"]).toEqual(["dashboards/gone"]));
    expect(await screen.findByText("Noch keine Favoriten.")).toBeInTheDocument();

    // Auf Vorgabe zurücksetzen takes the arrangement away as well.
    await user.click(screen.getByRole("button", { name: "Auf Vorgabe zurücksetzen" }));
    await waitFor(() => expect(areaNames()).toContain("Probe"));
    expect(areaNames()[0]).toBe("Dashboards");
    expect(before("Dashboards", "Probe")).toBe(true);
    expect(state.settings!["nav_order"]).toBeUndefined();
    expect(state.settings!["nav_hidden"]).toBeUndefined();
    expect(state.settings!["favorites"]).toBeUndefined();
  });
});
