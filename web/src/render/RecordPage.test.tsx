// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The record page (UI3): head, fields, timeline newest first with the changes of a version in
// one block, the switch to entries only, a new entry, a refused private one; the sidebar with
// the roles by name and the SLA clocks with their traffic light.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import type { Editor } from "@tiptap/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Clock, TimelineItem } from "@/api/client";
import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, me, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

import { blocks } from "./Timeline";
import { duration, light } from "./Sidebar";

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

const anna = "01a15a3c-8545-7691-b780-a00000000001";

// write puts text into the editor (UI7): jsdom has no layout, so typing does not reach
// ProseMirror; TipTap keeps its editor at the element. Typing is proven in Playwright.
function write(el: HTMLElement, text: string) {
  act(() => {
    (el as unknown as { editor: Editor }).editor.commands.insertContent(text);
  });
}
// seq gives every change of the fixtures an id of its own.
let seq = 0;

function change(
  n: number,
  version: number,
  action: string,
  field?: string,
  old?: unknown,
  value?: unknown,
): TimelineItem {
  const at = new Date(Date.UTC(2026, 9, 29, 8, n)).toISOString();
  return {
    kind: "change",
    at,
    actor: me.id,
    actor_name: "Pascal",
    change: {
      id: `01a15a3c-8545-7691-b780-c${String(++seq).padStart(11, "0")}`,
      version,
      action: action as "update",
      at,
      actor: me.id,
      ...(field ? { field, old, new: value } : {}),
    },
  };
}

function entry(n: number, body: string, isPrivate = false): TimelineItem {
  const at = new Date(Date.UTC(2026, 9, 29, 8, n)).toISOString();
  const id = `01a15a3c-8545-7691-b780-e${String(n).padStart(11, "0")}`;
  return {
    kind: "entry",
    at,
    actor: anna,
    actor_name: "Anna",
    entry: { id, body, format: "text", private: isPrivate, created_at: at, created_by: anna },
  };
}

const clock = (over: Partial<Clock>): Clock => ({
  id: "01a15a3c-8545-7691-b780-f00000000001",
  sla: "01a15a3c-8545-7691-b780-f00000000002",
  sla_name: "Lösung",
  state: "running",
  breached: false,
  levels: [],
  target_minutes: 240,
  elapsed_minutes: 30,
  remaining_minutes: 210,
  calendar: null,
  started_at: "2026-10-29T08:00:00Z",
  due_at: "2026-10-29T12:00:00Z",
  breached_at: null,
  stopped_at: null,
  ...over,
});

describe("RecordPage", () => {
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

  it("groups changes by version and turns the timeline round", () => {
    const b = blocks([
      change(1, 1, "create"),
      change(2, 2, "update", "title", "a", "b"),
      change(2, 2, "update", "kind", "question", "fault"),
      entry(3, "Hallo"),
      change(4, 3, "transition", "state", "new", "in_progress"),
    ]);
    expect(b.map((x) => x.kind)).toEqual(["changes", "entry", "changes", "changes"]);
    expect(b[2]).toMatchObject({ version: 2, changes: [{ field: "title" }, { field: "kind" }] });
  });

  it("says how the clocks stand", () => {
    expect(light(clock({}))).toBe("inTime");
    expect(light(clock({ remaining_minutes: 30 }))).toBe("close");
    expect(light(clock({ breached: true, remaining_minutes: -5 }))).toBe("breached");
    expect(light(clock({ state: "stopped" }))).toBe("met");
    expect(light(clock({ state: "paused" }))).toBe("paused");
    expect(duration(135)).toBe("2 h 15 min");
    expect(duration(-45)).toBe("45 min");
  });

  it("shows head, fields, timeline and sidebar, and adds an entry", async () => {
    const user = userEvent.setup();
    const rec = state.records[0]!;
    const id = String(rec["id"]);
    rec["actors"] = { requester: [{ type: "core.person", id: anna }] };
    state.persons = { [anna]: { id: anna, first_name: "Anna", last_name: "Berg" } };
    state.timeline = {
      [id]: [
        change(1, 1, "create"),
        change(2, 2, "update", "kind", "question", "fault"),
        change(2, 2, "update", "priority", 3, 2),
        entry(3, "Drucker im 3. Stock"),
        entry(4, "Intern: Ersatzteil bestellt", true),
      ],
    };
    state.clocks = {
      [id]: [
        clock({ sla_name: "Reaktion", remaining_minutes: 10, target_minutes: 60 }),
        clock({ id: "c2", sla_name: "Lösung", breached: true, remaining_minutes: -90 }),
      ],
    };
    const { container } = renderAt(`/types/probe.probe/${id}`);
    await screen.findByRole("heading", { name: "Probe 000001" });

    // Sidebar: the requester by name, two clocks with their light and time.
    const roles = await screen.findByRole("region", { name: "Rollen" });
    expect(await within(roles).findByText("Anna Berg")).toBeInTheDocument();
    const clocks = await screen.findByRole("region", { name: "SLA" });
    expect(clocks).toHaveTextContent("Reaktion");
    expect(clocks).toHaveTextContent("Knapp");
    expect(clocks).toHaveTextContent("Noch 10 min");
    expect(clocks).toHaveTextContent("Verletzt");
    expect(clocks).toHaveTextContent("Überfällig seit 1 h 30 min");

    // Timeline: newest first; the two changes of version 2 in one block, with labels.
    const timeline = screen.getByRole("region", { name: "Zeitleiste" });
    await within(timeline).findByText("Drucker im 3. Stock");
    // The blocks of the timeline, not the lines of changes inside them.
    const blocksShown = () => Array.from(timeline.querySelectorAll(":scope > ol > li"));
    const items = blocksShown();
    expect(items[0]).toHaveTextContent("Intern: Ersatzteil bestellt");
    expect(items[0]).toHaveTextContent("Privat");
    expect(items[1]).toHaveTextContent("Drucker im 3. Stock");
    expect(items[1]).toHaveTextContent("Anna");
    expect(items[2]).toHaveTextContent("Version 2");
    expect(items[2]).toHaveTextContent("Art: Frage → Störung");
    expect(items[2]).toHaveTextContent("Priorität: 3 → 2");
    expect(timeline).toHaveTextContent("Angelegt");
    await expectAccessible(container);

    // Entries only.
    await user.click(within(timeline).getByRole("button", { name: "Nur Einträge" }));
    expect(timeline).not.toHaveTextContent("Angelegt");
    expect(timeline).toHaveTextContent("Drucker im 3. Stock");

    // A new entry appears on top; a private one that is refused says why.
    write(
      within(timeline).getByRole("textbox", { name: "Neuer Eintrag" }),
      "Techniker kommt morgen",
    );
    await user.click(within(timeline).getByRole("button", { name: "Eintragen" }));
    await waitFor(() => expect(blocksShown()[0]).toHaveTextContent("Techniker kommt morgen"));
    await waitFor(() =>
      expect(within(timeline).getByRole("textbox", { name: "Neuer Eintrag" })).toHaveTextContent(
        "",
      ),
    );
    const sent = state.calls
      .filter((c) => c.path.endsWith("/timeline") && c.method === "POST")
      .at(-1)!;
    expect(JSON.parse(sent.body!)).toMatchObject({
      format: "html",
      body: "<p>Techniker kommt morgen</p>",
    });
    state.privateRefused = true;
    write(within(timeline).getByRole("textbox", { name: "Neuer Eintrag" }), "geheim");
    await user.click(within(timeline).getByLabelText("Privat (nur für Berechtigte)"));
    await user.click(within(timeline).getByRole("button", { name: "Eintragen" }));
    expect(await within(timeline).findByRole("alert")).toHaveTextContent("keine privaten Einträge");
  });
});
