// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The trash (UI63): the rows with type, title, time and person; restoring with "Öffnen" in the
// message; purging after a question that names the number, Esc stops; without purge no button;
// the line with the period of the trash; axe.
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
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

const loaded = { timeout: 5000 };

describe("TrashPage", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    state.may = ["read", "create", "update", "delete", "purge"];
    // Two probes in the trash, the second deleted last.
    state.trash = state.records.splice(0, 2);
    state.trash[0]!["deleted_at"] = "2026-11-18T09:00:00Z";
    state.trash[1]!["deleted_at"] = "2026-11-19T09:00:00Z";
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists the trash, restores with Öffnen and purges after a question", async () => {
    const user = userEvent.setup();
    const [first, second] = state.trash!;
    const router = renderAt("/trash");
    await screen.findByRole("heading", { name: "Papierkorb", level: 1 });
    expect(await screen.findByText(/bleibt 30 Tage hier/, {}, loaded)).toBeInTheDocument();
    const table = await screen.findByRole("table", {}, loaded);
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    // Newest first, with type and person.
    expect(rows[0]).toHaveTextContent(String(second!["title"]));
    expect(rows[0]).toHaveTextContent("Probe");
    expect(rows[0]).toHaveTextContent("Kollegin");
    await expectAccessible(document.body);

    // Restore the newer one: a green message with "Öffnen" that leads to it.
    await user.click(
      screen.getByRole("checkbox", { name: `„${String(second!["title"])}“ auswählen` }),
    );
    await user.click(screen.getByRole("button", { name: "Wiederherstellen" }));
    const note = await screen.findByText("1 wiederhergestellt");
    expect(state.trash!.map((r) => r["id"])).toEqual([first!["id"]]);
    await user.click(
      within(note.closest("[role=status],[role=alert]") as HTMLElement).getByRole("button", {
        name: "Öffnen",
      }),
    );
    await waitFor(() =>
      expect(router.state.location.pathname).toBe(`/types/probe.probe/${String(second!["id"])}`),
    );

    // Purge the other: the question names the number; Esc stops, Endgültig löschen goes on.
    void router.navigate({ to: "/trash" });
    await user.click(
      await screen.findByRole("checkbox", { name: `„${String(first!["title"])}“ auswählen` }),
    );
    await user.click(screen.getByRole("button", { name: "Endgültig löschen" }));
    const ask = await screen.findByRole("alertdialog", { name: "1 Eintrag endgültig löschen?" });
    expect(ask).toHaveTextContent("nicht rückgängig");
    expect(within(ask).getByRole("button", { name: "Endgültig löschen" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(state.trash).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "Endgültig löschen" }));
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Endgültig löschen",
      }),
    );
    expect(await screen.findByText("1 endgültig gelöscht")).toBeInTheDocument();
    expect(state.trash).toEqual([]);
    expect(await screen.findByText("Der Papierkorb ist leer.")).toBeInTheDocument();
  });

  it("offers no purge without the right and says when the trash keeps for ever", async () => {
    const user = userEvent.setup();
    state.may = ["read", "delete"];
    state.orgSettings = { root: { "trash.keep_days": 0 } };
    renderAt("/trash");
    expect(
      await screen.findByText(
        "Gelöschtes bleibt hier, bis es jemand endgültig löscht.",
        {},
        loaded,
      ),
    ).toBeInTheDocument();
    await user.click(await screen.findByRole("checkbox", { name: "Alle auswählen" }, loaded));
    expect(screen.getByRole("button", { name: "Wiederherstellen" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Endgültig löschen" })).toBeNull();
  });

  it("filters by the title", async () => {
    const user = userEvent.setup();
    const [first] = state.trash!;
    renderAt("/trash");
    await screen.findByRole("table", {}, loaded);
    await user.type(screen.getByLabelText("Titel enthält"), String(first!["title"]));
    await waitFor(() => expect(screen.getAllByRole("row")).toHaveLength(2));
    expect(screen.getAllByRole("row")[1]).toHaveTextContent(String(first!["title"]));
  });
});
