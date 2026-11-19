// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// "Meine Vorgänge" (UI43, UI plan E52): one list over the types, the last changed first, the
// number of the open ones at the heading, the hint when it is the person's turn, "Offen" and
// "Alle", 20 at a time.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { RecordData, TypeDefinition } from "@/api/client";
import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, me, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

const person = "01a15a3c-8545-7691-b780-a00000000077";

const states: TypeDefinition["states"] = [
  { name: "new", label: { de: "Neu", en: "New" }, initial: true, category: "open" },
  { name: "waiting", label: { de: "Wartet", en: "Waiting" }, category: "waiting" },
  { name: "resolved", label: { de: "Gelöst", en: "Resolved" }, category: "done" },
  { name: "closed", label: { de: "Geschlossen", en: "Closed" }, category: "done" },
];

// ticket is a type of the service desk as the portal sees it: a running number, a reason to
// wait, the requester who confirms the solution.
function ticket(name: string, label: string, prefix: string): TypeDefinition {
  return {
    name,
    package: name,
    label: { de: label, en: label },
    title_field: "title",
    fields: [
      { name: "title", type: "text", label: { de: "Titel", en: "Title" } },
      { name: "number", type: "sequence", prefix, label: { de: "Nummer", en: "Number" } },
      {
        name: "wait_reason",
        type: "reference",
        to: "core.wait_reason",
        label: { de: "Wartegrund", en: "Reason to wait" },
      },
    ],
    states,
    transitions: [
      {
        name: "confirm",
        label: { de: "Lösung bestätigen", en: "Confirm the solution" },
        from: ["resolved"],
        to: "closed",
        by: ["requester"],
      },
    ],
    actors: [{ name: "requester", to: ["core.person"] }],
    portal: { fields: ["title"] },
  };
}

function mineRecord(id: string, values: RecordData): RecordData {
  return {
    id,
    version: 1,
    created_by: me.id,
    actors: { requester: [{ type: "core.person", id: person }] },
    ...values,
  };
}

function renderAt(path: string) {
  const queryClient = createQueryClient();
  const router = createAppRouter({ queryClient, history: memoryHistory(path) });
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

// The list asks for the person, the types and then each type twice (count and page): about a
// second in jsdom, more on a shared ki-lab (PL74 CI, 2026-11-19); the default of 1 s is too tight.
const loaded = { timeout: 5000 };

describe("Meine Vorgänge", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(0);
    state.signedIn = true;
    state.workplace = false;
    state.personId = person;
    state.extraTypes = [
      ticket("incident", "Störung", "INC-"),
      ticket("request", "Anfrage", "REQ-"),
    ];
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists one incident and two requests, the last changed first, with the hints", async () => {
    const user = userEvent.setup();
    state.extraRecords = {
      "incident.incident": [
        mineRecord("i1", {
          title: "Drucker streikt",
          number: 3,
          state: "waiting",
          "wait_reason.awaits_requester": true,
          updated_at: "2026-11-16T09:00:00Z",
        }),
      ],
      "request.request": [
        mineRecord("r1", {
          title: "Neuer Monitor",
          number: 2,
          state: "resolved",
          updated_at: "2026-11-16T10:00:00Z",
        }),
        mineRecord("r2", {
          title: "Zugang Laufwerk",
          number: 1,
          state: "new",
          updated_at: "2026-11-16T08:00:00Z",
        }),
        mineRecord("r3", {
          title: "Alte Maus",
          number: 0,
          state: "closed",
          updated_at: "2026-11-16T07:00:00Z",
        }),
      ],
    };
    const { container } = renderAt("/portal");
    const list = await screen.findByRole("region", { name: "Meine Vorgänge" });
    expect(await within(list).findByText("3 offen", {}, loaded)).toBeInTheDocument();
    const rows = await within(list).findAllByRole("listitem");
    expect(rows.map((r) => within(r).getByRole("link").textContent)).toEqual([
      "Neuer Monitor",
      "Drucker streikt",
      "Zugang Laufwerk",
    ]);
    // A solved request waits for the confirmation, an incident for an answer.
    expect(within(rows[0]!).getByText("Lösung bestätigen")).toBeInTheDocument();
    expect(within(rows[0]!).getByText(/^REQ-2 · Anfrage · /)).toBeInTheDocument();
    expect(within(rows[1]!).getByText("Rückfrage beantworten")).toBeInTheDocument();
    expect(within(rows[1]!).getByText("Wartet")).toBeInTheDocument();
    expect(within(rows[2]!).queryByText(/beantworten|bestätigen/)).toBeNull();
    // No card per type and no "Neu" of its own any more.
    expect(screen.queryByRole("region", { name: "Anfrage" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Neu" })).toBeNull();
    await expectAccessible(container);

    // "Alle" shows the closed ones too; the number at the heading stays that of the open.
    await user.click(within(list).getByRole("button", { name: "Alle" }));
    expect(within(list).getByRole("button", { name: "Alle" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(await within(list).findByRole("link", { name: "Alte Maus" })).toBeInTheDocument();
    expect(within(list).getAllByRole("listitem")).toHaveLength(4);
    expect(within(list).getByText("3 offen")).toBeInTheDocument();
  });

  it("shows 20 and the next 20 on demand", async () => {
    const user = userEvent.setup();
    state.extraRecords = {
      "request.request": Array.from({ length: 25 }, (_, i) =>
        mineRecord(`r${i}`, {
          title: `Anfrage ${i}`,
          number: i,
          state: "new",
          updated_at: `2026-11-16T${String(10 + (i % 10)).padStart(2, "0")}:${String(i).padStart(2, "0")}:00Z`,
        }),
      ),
    };
    renderAt("/portal");
    const list = await screen.findByRole("region", { name: "Meine Vorgänge" });
    expect(await within(list).findByText("25 offen", {}, loaded)).toBeInTheDocument();
    expect(await within(list).findAllByRole("listitem")).toHaveLength(20);
    await user.click(within(list).getByRole("button", { name: "Weitere anzeigen" }));
    expect(await within(list).findByRole("link", { name: "Anfrage 24" })).toBeInTheDocument();
    expect(within(list).getAllByRole("listitem")).toHaveLength(25);
    expect(within(list).queryByRole("button", { name: "Weitere anzeigen" })).toBeNull();
  });

  it("says when nothing is open", async () => {
    renderAt("/portal");
    const list = await screen.findByRole("region", { name: "Meine Vorgänge" });
    expect(await within(list).findByText("Nichts offen.", {}, loaded)).toBeInTheDocument();
    expect(within(list).getByText("0 offen")).toBeInTheDocument();
  });
});
