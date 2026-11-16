// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The header of the workplace (UI35, UI plan E41, E47): no labelled selects any more; the bell
// shows the unread of the inbox and opens a window of the newest entries; the account circle
// opens a window with mode, language, Meine Einstellungen, the portal and signing out. Esc
// closes and gives the focus back.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { initials } from "@/app/Header";
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

const record = "01a15a3c-8545-7691-b780-000000000001";

describe("Header", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    state.inbox = [
      {
        id: "01a1e3a0-0000-7000-8000-000000000001",
        kind: "create",
        type_name: "probe.probe",
        record_id: record,
        actor: "Anna Berg",
        subject: "Neue Probe: Drucker",
        body: "",
        created_at: "2026-11-16T09:00:00Z",
      },
      {
        id: "01a1e3a0-0000-7000-8000-000000000002",
        kind: "update",
        type_name: "probe.probe",
        record_id: record,
        actor: "Ben Kahl",
        subject: "Probe geändert",
        body: "",
        created_at: "2026-11-15T09:00:00Z",
        read_at: "2026-11-15T10:00:00Z",
      },
    ];
    initI18n("de");
    document.documentElement.dataset.colorScheme = "light";
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("makes initials of a name", () => {
    expect(initials("Mara Koch")).toBe("MK");
    expect(initials("Pascal")).toBe("P");
    expect(initials("  anna  von  berg ")).toBe("AB");
    expect(initials("")).toBe("?");
  });

  it("opens the newest entries at the bell and marks one read on opening it", async () => {
    const user = userEvent.setup();
    const view = renderAt("/");
    await screen.findByRole("heading", { name: "Willkommen, Pascal" });
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    const bell = await screen.findByRole("button", { name: "Benachrichtigungen, 1 neu" });
    await user.click(bell);
    const panel = screen.getByRole("dialog", { name: "Benachrichtigungen" });
    const links = within(panel).getAllByRole("link");
    expect(links[0]).toHaveTextContent("Ungelesen: Neue Probe: Drucker");
    expect(links[0]).toHaveTextContent("Anna Berg");
    expect(links[1]).toHaveTextContent("Probe geändert");
    expect(links[1]).not.toHaveTextContent("Ungelesen");
    await expectAccessible(view.container);
    await user.keyboard("{Escape}");
    expect(panel).not.toBeInTheDocument();
    expect(bell).toHaveFocus();

    await user.click(bell);
    await user.click(screen.getAllByRole("link", { name: /Neue Probe: Drucker/ })[0]!);
    await waitFor(() =>
      expect(view.router.state.location.pathname).toBe(`/types/probe.probe/${record}`),
    );
    expect(await screen.findByRole("button", { name: "Benachrichtigungen" })).toBeInTheDocument();
    const read = state.calls.find((c) => c.path === "/api/v1/me/inbox:read");
    expect(read?.body).toBe(JSON.stringify({ ids: ["01a1e3a0-0000-7000-8000-000000000001"] }));
  });

  it("marks everything read", async () => {
    const user = userEvent.setup();
    renderAt("/");
    await user.click(await screen.findByRole("button", { name: "Benachrichtigungen, 1 neu" }));
    await user.click(screen.getByRole("button", { name: "Alle gelesen" }));
    expect(await screen.findByRole("button", { name: "Alle gelesen" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Benachrichtigungen" })).toBeInTheDocument();
  });

  it("sets mode and language in the account window, which Esc closes", async () => {
    const user = userEvent.setup();
    const view = renderAt("/");
    const account = await screen.findByRole("button", { name: "Konto von Pascal" });
    await user.click(account);
    const panel = screen.getByRole("dialog", { name: "Konto" });
    expect(within(panel).getByRole("link", { name: "Meine Einstellungen" })).toHaveAttribute(
      "href",
      "/settings",
    );
    expect(within(panel).getByRole("link", { name: "Zum Portal" })).toHaveAttribute(
      "href",
      "/portal",
    );
    await expectAccessible(view.container);

    await user.click(within(panel).getByRole("button", { name: "Dunkel" }));
    expect(document.documentElement.dataset.colorScheme).toBe("dark");
    expect(within(panel).getByRole("button", { name: "Dunkel" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await waitFor(() =>
      expect(
        state.calls.some(
          (c) =>
            c.path === "/api/v1/me/settings" &&
            c.method === "PATCH" &&
            c.body?.includes('"color_scheme":"dark"'),
        ),
      ).toBe(true),
    );

    await user.click(within(panel).getByRole("button", { name: "English" }));
    expect(await screen.findByRole("dialog", { name: "Account" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Account" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Account of Pascal" })).toHaveFocus();
  });
});
