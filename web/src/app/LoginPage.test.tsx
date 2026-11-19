// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The sign-in page through the real router with a fake API: without a session the shell
// redirects to /login; the page is accessible (axe); empty fields are named; a wrong password
// shows the failure; a right one leads to the shell with the navigation.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
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

describe("LoginPage", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("is where the shell sends you without a session, and is accessible", async () => {
    const { router, container } = renderAt("/");
    await screen.findByRole("heading", { name: "Anmelden" });
    expect(router.state.location.pathname).toBe("/login");
    await expectAccessible(container);
  });

  it("names empty fields and reports a wrong password", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/login");
    await user.click(await screen.findByRole("button", { name: "Anmelden" }));
    expect(await screen.findAllByText("Bitte ausfüllen.")).toHaveLength(2);
    expect(screen.getByLabelText("Benutzername")).toHaveAttribute("aria-invalid", "true");
    await expectAccessible(container);

    await user.type(screen.getByLabelText("Benutzername"), "pascal");
    await user.type(screen.getByLabelText("Kennwort"), "wrong");
    await user.click(screen.getByRole("button", { name: "Anmelden" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Benutzername oder Kennwort ist falsch.",
    );

    // Locked after failed sign-ins: the minutes left from Retry-After (840 s); a full instance
    // asks to try again in a moment (AP6).
    await user.clear(screen.getByLabelText("Benutzername"));
    await user.type(screen.getByLabelText("Benutzername"), "gesperrt");
    await user.click(screen.getByRole("button", { name: "Anmelden" }));
    expect(
      await screen.findByText("Zu viele Fehlversuche. Bitte in 14 Minuten erneut versuchen."),
    ).toBeInTheDocument();
    await user.clear(screen.getByLabelText("Benutzername"));
    await user.type(screen.getByLabelText("Benutzername"), "voll");
    await user.click(screen.getByRole("button", { name: "Anmelden" }));
    expect(
      await screen.findByText(
        "Gerade melden sich sehr viele an. Bitte gleich noch einmal versuchen.",
      ),
    ).toBeInTheDocument();
  });

  it("leads to the shell with the navigation and signs out again", async () => {
    const user = userEvent.setup();
    const { router, container } = renderAt("/login");
    await user.type(await screen.findByLabelText("Benutzername"), "pascal");
    await user.type(screen.getByLabelText("Kennwort"), "Secret-1!");
    await user.click(screen.getByRole("button", { name: "Anmelden" }));

    await screen.findByRole("heading", { name: "Willkommen, Pascal" });
    expect(router.state.location.pathname).toBe("/");
    expect(await screen.findByRole("link", { name: "Probe" })).toHaveAttribute(
      "href",
      "/types/probe.probe",
    );
    expect(screen.getByRole("button", { name: "Konto von Pascal" })).toBeInTheDocument();
    await expectAccessible(container);

    await user.click(screen.getByRole("link", { name: "Probe" }));
    await screen.findByRole("heading", { name: "Probe" });
    await screen.findByText("3 Datensätze");

    await user.click(screen.getByRole("button", { name: "Konto von Pascal" }));
    await user.click(screen.getByRole("button", { name: "Abmelden" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(state.signedIn).toBe(false);
  });
});
