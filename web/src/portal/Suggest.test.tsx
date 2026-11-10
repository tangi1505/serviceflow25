// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Knowledge in the portal (SD18, variant 1A): while a user writes, suggestions appear beside
// the form; one that helped ends the report without creating anything. Under an article the
// user says whether it helped and comments on it.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Editor } from "@tiptap/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { articleType, fakeApi, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

function renderAt(path: string) {
  const queryClient = createQueryClient();
  const router = createAppRouter({ queryClient, history: memoryHistory(path) });
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

const article = "01a15a3c-8545-7691-b780-c00000000004";

describe("Knowledge in the portal", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.workplace = false;
    state.extraTypes = [articleType];
    state.extraRecords = {
      "knowledge.article": [
        {
          id: article,
          title: "VPN verbindet nicht",
          body: "<p>Starten Sie den VPN-Client neu.</p>",
          updated_at: "2026-11-04T09:00:00Z",
          version: 1,
        },
      ],
    };
    state.suggest = [
      {
        type: "knowledge.article",
        label: { de: "Wissen" },
        relation: "knowledge.uses",
        hits: [
          {
            id: article,
            title: "VPN verbindet nicht",
            snippet: "Wenn die Verbindung <mark>abbricht</mark>",
            rank: 1,
            rating: { yes: 0, no: 0, mine: null },
            uses: 0,
          },
        ],
      },
    ];
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("suggests while the user writes and ends when one helped", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/portal/probe.probe/new");
    const aside = await screen.findByRole("complementary", { name: "Vielleicht hilft das" });
    expect(aside).toHaveTextContent("Sobald Sie schreiben");
    await user.type(screen.getByLabelText(/^Titel/), "VPN bricht ab");
    const hit = await within(aside).findByRole(
      "button",
      { name: "VPN verbindet nicht" },
      { timeout: 3000 },
    );
    const asked = state.calls
      .filter((c) => c.path === "/api/v1/records/probe.probe:suggest")
      .at(-1)!;
    expect(JSON.parse(asked.body!)).toEqual({ values: { title: "VPN bricht ab" }, limit: 3 });
    expect(hit).toHaveAttribute("aria-expanded", "false");
    await expectAccessible(container);

    // Open in place, then "Das hat geholfen": a yes, nothing created.
    await user.click(hit);
    expect(hit).toHaveAttribute("aria-expanded", "true");
    expect(await within(aside).findByText("Starten Sie den VPN-Client neu.")).toBeInTheDocument();
    await user.click(within(aside).getByRole("button", { name: "Das hat geholfen" }));
    expect(await screen.findByText("Gut, dass es geklappt hat.")).toBeInTheDocument();
    expect(state.ratings?.[article]).toEqual({ yes: 1, no: 0, mine: true });
    expect(
      state.calls.some((c) => c.method === "POST" && c.path === "/api/v1/records/probe.probe"),
    ).toBe(false);
    expect(screen.queryByRole("complementary")).toBeNull();

    // Report anyway: the form is back with what was written.
    await user.click(screen.getByRole("button", { name: "Doch melden" }));
    expect(screen.getByLabelText(/^Titel/)).toHaveValue("VPN bricht ab");
  });

  it("rates an article and comments on it", async () => {
    const user = userEvent.setup();
    const { container } = renderAt(`/portal/read/knowledge.article/${article}`);
    const box = await screen.findByRole("region", { name: "Hat Ihnen das geholfen?" });
    expect(await within(box).findByText("Noch nicht bewertet.")).toBeInTheDocument();
    await user.click(within(box).getByRole("button", { name: "Ja" }));
    await waitFor(() =>
      expect(within(box).getByRole("button", { name: "Ja" })).toHaveAttribute(
        "aria-pressed",
        "true",
      ),
    );
    expect(box).toHaveTextContent(
      "Danke für Ihre Rückmeldung, Sie können sie ändern. 1 von 1 fanden das hilfreich.",
    );
    await user.click(within(box).getByRole("button", { name: "Nein" }));
    await waitFor(() => expect(box).toHaveTextContent("0 von 1 fanden das hilfreich."));

    const comments = screen.getByRole("region", { name: "Kommentare" });
    expect(await within(comments).findByText("Noch keine Kommentare.")).toBeInTheDocument();
    const editor = within(comments).getByRole("textbox", { name: "Ihr Kommentar" });
    act(() => {
      (editor as unknown as { editor: Editor }).editor.commands.insertContent("Schritt 2 reicht.");
    });
    await user.click(within(comments).getByRole("button", { name: "Kommentieren" }));
    expect(await within(comments).findByText("Schritt 2 reicht.")).toBeInTheDocument();
    const sent = state.calls.find(
      (c) => c.method === "POST" && c.path.endsWith(`${article}/timeline`),
    )!;
    expect(JSON.parse(sent.body!)).toMatchObject({ format: "html", private: false });
    await expectAccessible(container);
  });
});
