// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The tab "Fassungen" (SD16b, variant A): the versions newest first with who and what; the two
// newest compared by default, the words of a text struck out and underlined, a choice by its
// label; another version chosen by a click; restoring asks first and sends the version seen.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";
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

describe("Versions", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.records[0]!["version"] = 3;
    state.versions = [
      {
        version: 1,
        at: "2026-10-28T08:12:00Z",
        actions: ["create"],
        changed: ["title"],
        actor_name: "Anna Weber",
      },
      {
        version: 2,
        at: "2026-10-30T13:40:00Z",
        actions: ["update"],
        changed: ["kind"],
        actor_name: "Anna Weber",
      },
      {
        version: 3,
        at: "2026-11-04T07:05:00Z",
        actions: ["update"],
        changed: ["title"],
        actor_name: "M. Muster",
      },
    ];
    state.comparisons = {
      "2-3": {
        from: 2,
        to: 3,
        changes: [
          { field: "title", from: "Drucker druckt leer", to: "Drucker druckt nur leere Seiten" },
        ],
      },
      "1-3": {
        from: 1,
        to: 3,
        changes: [
          { field: "title", from: "Drucker druckt leer", to: "Drucker druckt nur leere Seiten" },
          { field: "kind", from: "question", to: "fault" },
        ],
      },
    };
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("compares two versions and restores the older after asking", async () => {
    const user = userEvent.setup();
    const { container } = renderAt(`/types/probe.probe/${state.records[0]!.id}`);
    const tabs = await screen.findByRole("tablist", { name: "Bereiche des Datensatzes" });
    await user.click(within(tabs).getByRole("tab", { name: "Fassungen" }));

    const list = await screen.findByRole("list", { name: "Fassungen" });
    const items = within(list).getAllByRole("button");
    expect(items.map((b) => b.textContent)).toEqual([
      expect.stringContaining("Fassung 3"),
      expect.stringContaining("Fassung 2"),
      expect.stringContaining("Fassung 1"),
    ]);
    expect(items[0]).toHaveTextContent("M. Muster · Titel");
    expect(items[2]).toHaveTextContent("Anna Weber · Angelegt");
    expect(items[0]).toHaveAttribute("aria-pressed", "true");
    expect(items[1]).toHaveAttribute("aria-pressed", "true");

    // The two newest: the words that went and came.
    expect(
      await screen.findByRole("heading", { name: "Fassung 2 und Fassung 3" }),
    ).toBeInTheDocument();
    const ins = await screen.findByText("nur leere Seiten");
    expect(ins.tagName).toBe("INS");
    expect(screen.getByText("leer", { selector: "del" })).toBeInTheDocument();

    // Version 1 instead of 2: the choice by its label.
    await user.click(items[2]!);
    expect(
      await screen.findByRole("heading", { name: "Fassung 1 und Fassung 3" }),
    ).toBeInTheDocument();
    const table = await screen.findByRole("table");
    expect(await within(table).findByText("Störung")).toBeInTheDocument();
    expect(within(table).getByText("Frage").closest("del")).not.toBeNull();
    await expectAccessible(container);

    // Restoring asks first; the answer sends the version seen.
    await user.click(screen.getByRole("button", { name: "Fassung 1 wiederherstellen" }));
    const ask = screen.getByRole("group", { name: "Fassung 1 wiederherstellen?" });
    expect(ask).toHaveTextContent("Das legt Fassung 4");
    await user.click(within(ask).getByRole("button", { name: "Wiederherstellen" }));
    await vi.waitFor(() =>
      expect(
        state.calls.some((c) => c.method === "POST" && c.path.endsWith("/versions/1:restore")),
      ).toBe(true),
    );
    expect(state.records[0]!["version"]).toBe(4);
  });

  it("says when someone saved in between", async () => {
    const user = userEvent.setup();
    state.restoreConflict = true;
    renderAt(`/types/probe.probe/${state.records[0]!.id}`);
    const tabs = await screen.findByRole("tablist", { name: "Bereiche des Datensatzes" });
    await user.click(within(tabs).getByRole("tab", { name: "Fassungen" }));
    await user.click(await screen.findByRole("button", { name: "Fassung 2 wiederherstellen" }));
    await user.click(screen.getByRole("button", { name: "Wiederherstellen" }));
    expect(await screen.findByText(/inzwischen geändert/)).toBeInTheDocument();
  });
});
