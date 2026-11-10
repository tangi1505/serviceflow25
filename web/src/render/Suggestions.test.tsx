// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The tab of suggestions on a record page (SD18, SD-Plan E12): a type that names this one
// under suggest gets a tab labelled as it says; its hits link and are used, their values go
// into the form unsaved; snippets show their marks as text.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { TypeDefinition } from "@/api/client";
import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { articleType, fakeApi, newState, probeType, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

import { marked, suggesters } from "./Suggestions";

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

describe("suggesters and marks", () => {
  it("finds the types suggested by name or by trait", () => {
    const ticket: TypeDefinition = {
      name: "incident",
      package: "incident",
      fields: [],
      traits: ["sd.ticket"],
    };
    const own: TypeDefinition = { name: "case", package: "sd", fields: [], traits: ["ticket"] };
    const other: TypeDefinition = { name: "server", package: "cmdb", fields: [] };
    const byTrait = {
      ...articleType,
      suggest: { ...articleType.suggest!, for: ["trait:sd.ticket"] },
    };
    expect(suggesters([byTrait], ticket)).toEqual([byTrait]);
    expect(suggesters([byTrait], own)).toEqual([byTrait]);
    expect(suggesters([byTrait], other)).toEqual([]);
    expect(suggesters([articleType], { ...probeType })).toEqual([articleType]);
  });

  it("shows the marks of a snippet and no markup of it", () => {
    const { container } = render(
      <p>{marked('<p>Wenn die <mark>Verbindung</mark> <img src=x onerror="alert(1)">abbricht')}</p>,
    );
    expect(container.querySelector("mark")).toHaveTextContent("Verbindung");
    expect(container.querySelector("img")).toBeNull();
    expect(container).toHaveTextContent("Wenn die Verbindung abbricht");
  });
});

describe("Tab of suggestions", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    state.extraTypes = [articleType];
    state.extraRecords = {
      "knowledge.article": [
        { id: article, title: "VPN verbindet nicht", body: "<p>Neu starten.</p>", version: 1 },
      ],
    };
    state.suggest = [
      {
        type: "knowledge.article",
        label: { de: "Wissen" },
        relation: "knowledge.uses",
        apply: { label: { de: "Als Titel übernehmen" }, set: { title: "@title", kind: "fault" } },
        hits: [
          {
            id: article,
            title: "VPN verbindet nicht",
            snippet: "Wenn die <mark>Verbindung</mark> abbricht",
            rank: 1,
            rating: { yes: 12, no: 3, mine: null },
            uses: 7,
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

  it("links a hit and puts its values into the form", async () => {
    const user = userEvent.setup();
    const id = state.records[0]!.id as string;
    const { container } = renderAt(`/types/probe.probe/${id}`);
    const tab = await screen.findByRole("tab", { name: "Wissen" });
    const tabs = screen.getAllByRole("tab").map((t) => t.textContent);
    expect(tabs.slice(0, 2)).toEqual(["Bezüge", "Wissen"]);
    await user.click(tab);
    const panel = screen.getByRole("tabpanel");
    expect(
      await within(panel).findByRole("link", { name: "VPN verbindet nicht" }),
    ).toBeInTheDocument();
    expect(panel).toHaveTextContent("12 von 15 hilfreich · 7-mal genutzt");
    expect(within(panel).getByText("Verbindung", { selector: "mark" })).toBeInTheDocument();
    const asked = state.calls.find((c) => c.path === "/api/v1/records/probe.probe:suggest")!;
    expect(JSON.parse(asked.body!)).toMatchObject({
      values: { title: "Probe 000001" },
      id,
      limit: 5,
    });
    await expectAccessible(container);

    // Link: a relation from the record to the hit; the hit then says so.
    await user.click(
      within(panel).getByRole("button", { name: "„VPN verbindet nicht“ verknüpfen" }),
    );
    await waitFor(() => expect(within(panel).getByText("Verknüpft")).toBeInTheDocument());
    expect(state.relations).toEqual([
      expect.objectContaining({
        kind: "knowledge.uses",
        from: { type: "probe.probe", id },
        to: { type: "knowledge.article", id: article },
      }),
    ]);

    // Use: the values come into the form, unsaved; it stays linked once.
    await user.click(
      within(panel).getByRole("button", { name: "Als Titel übernehmen: VPN verbindet nicht" }),
    );
    await waitFor(() => expect(screen.getByLabelText(/^Titel/)).toHaveValue("VPN verbindet nicht"));
    expect(screen.getByLabelText(/^Art/)).toHaveValue("fault");
    expect(panel).toHaveTextContent("im Formular noch nicht gespeichert");
    expect(state.relations).toHaveLength(1);
    expect(state.calls.some((c) => c.method === "PATCH")).toBe(false);

    // An own search asks with the text.
    await user.type(within(panel).getByLabelText("Selbst suchen"), "Drucker");
    await user.click(within(panel).getByRole("button", { name: "Suchen" }));
    await waitFor(() =>
      expect(
        state.calls.some(
          (c) => c.path.endsWith(":suggest") && JSON.parse(c.body!).text === "Drucker",
        ),
      ).toBe(true),
    );
    expect(panel).toHaveTextContent("Eigene Suche: „Drucker“.");
  });
});
