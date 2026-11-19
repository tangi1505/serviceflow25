// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The list and the record page with the keyboard (UI53, UI-Plan E55): arrows change the row,
// Enter shows the preview, Space chooses, Umschalt Enter opens; on the record page Strg S saves
// (and stops the browser's "Speichern unter"), Strg Enter sends the answer of the timeline.
import type { Editor } from "@tiptap/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";

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

describe("Keyboard of list and record", () => {
  let state: FakeState;
  let width: number;
  beforeEach(() => {
    state = newState(5);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
    // Room for the preview beside the list (from 1100 px, UI30).
    width = window.innerWidth;
    Object.defineProperty(window, "innerWidth", {
      value: 1400,
      configurable: true,
      writable: true,
    });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    Object.defineProperty(window, "innerWidth", {
      value: width,
      configurable: true,
      writable: true,
    });
  });

  it("moves through the list, previews, chooses and opens", async () => {
    const user = userEvent.setup();
    const { router } = renderAt("/types/probe.probe");
    await screen.findByText("5 Datensätze");
    // Sorted by the last change, the newest first: Probe 000005 leads.
    const rows = () => screen.getAllByRole("row").filter((r) => r.closest("tbody"));
    rows()[0]!.focus();
    await user.keyboard("{ArrowDown}{ArrowDown}");
    const third = rows()[2]!;
    expect(third).toHaveFocus();
    const name = within(third).getAllByRole("cell")[1]!.textContent ?? "";

    await user.keyboard("{Enter}");
    const preview = await screen.findByRole("complementary", { name: "Vorschau" });
    expect(within(preview).getByRole("heading", { name })).toBeInTheDocument();
    // Esc closes the preview (UI52).
    third.focus();
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("complementary", { name: "Vorschau" })).toBeNull(),
    );

    // Space chooses the row: the bar of mass actions comes.
    third.focus();
    await user.keyboard(" ");
    expect(await screen.findByRole("group", { name: "1 ausgewählt" })).toBeInTheDocument();
    await user.keyboard("{ArrowUp}");
    expect(rows()[1]).toHaveFocus();

    // Umschalt Enter opens the record.
    await user.keyboard("{Shift>}{Enter}{/Shift}");
    await waitFor(() =>
      expect(router.state.location.pathname).toMatch(/^\/types\/probe\.probe\/.+/),
    );
  });

  it("saves with Strg S and sends the answer with Strg Enter", async () => {
    const user = userEvent.setup();
    const first = state.records[0]!;
    renderAt(`/types/probe.probe/${String(first["id"])}`);
    const title = await screen.findByLabelText(/Titel/);
    await user.clear(title);
    await user.type(title, "Mit Strg S");
    // The key event is cancelled, so the browser opens no "Speichern unter".
    const allowed = fireEvent.keyDown(title, { key: "s", ctrlKey: true });
    expect(allowed).toBe(false);
    expect(await screen.findByText("Gespeichert.")).toBeInTheDocument();
    expect(first).toMatchObject({ title: "Mit Strg S" });

    await user.click(screen.getByRole("tab", { name: "Zeitleiste" }));
    const timeline = screen.getByRole("region", { name: "Zeitleiste" });
    const editor = within(timeline).getByRole("textbox", { name: "Neuer Eintrag" });
    act(() => {
      (editor as unknown as { editor: Editor }).editor.commands.insertContent("Per Tastatur");
    });
    fireEvent.keyDown(editor, { key: "Enter", ctrlKey: true });
    await waitFor(() => {
      const sent = state.calls.filter((c) => c.path.endsWith("/timeline") && c.method === "POST");
      expect(sent).toHaveLength(1);
      expect(JSON.parse(sent[0]!.body!)).toMatchObject({ body: "<p>Per Tastatur</p>" });
    });
  });
});
