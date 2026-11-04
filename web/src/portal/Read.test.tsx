// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The reading area of the portal (SD16a, variant B): "Wissen" in the head beside "Meine
// Vorgänge"; the tree of categories narrows to an entry and what lies below it; the search
// finds by the words; an article shows its category by name and offers to report; a reference
// on the page of one's own record shows the name of its record (finding of the click test).
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { TypeDefinition } from "@/api/client";
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

const id = (n: number) => `01a15a3c-8545-7691-b780-c000000000${String(n).padStart(2, "0")}`;
const [network, vpn, printer, vpnArticle, printerArticle, ticket] = [1, 2, 3, 4, 5, 6].map(id);

const categoryType: TypeDefinition = {
  name: "category",
  package: "sd",
  label: { de: "Kategorie" },
  title_field: "name",
  traits: ["list"],
  fields: [
    { name: "name", type: "text", label: { de: "Name" } },
    { name: "parent", type: "reference", label: { de: "Übergeordnet" }, to: "sd.category" },
  ],
};

const articleType: TypeDefinition = {
  name: "article",
  package: "knowledge",
  label: { de: "Artikel" },
  title_field: "title",
  fields: [
    { name: "title", type: "text", label: { de: "Titel" } },
    { name: "body", type: "text", format: "rich", label: { de: "Text" } },
    { name: "category", type: "reference", label: { de: "Kategorie" }, to: "sd.category" },
  ],
  portal: {
    read: {
      label: { de: "Wissen", en: "Knowledge" },
      fields: ["category", "body"],
      group: "category",
    },
  },
};

// A type users create in the portal, with a reference among its fields.
const ticketType: TypeDefinition = {
  name: "ticket",
  package: "demo",
  label: { de: "Meldung" },
  title_field: "title",
  fields: [
    { name: "title", type: "text", label: { de: "Titel" } },
    { name: "category", type: "reference", label: { de: "Kategorie" }, to: "sd.category" },
  ],
  portal: { fields: ["title", "category"] },
};

describe("Reading area of the portal", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.workplace = false;
    state.extraTypes = [categoryType, articleType, ticketType];
    state.extraRecords = {
      "sd.category": [
        { id: network, name: "Netz und Telefonie", parent: null, position: 1 },
        { id: vpn, name: "VPN", parent: network, position: 1 },
        { id: printer, name: "Drucker", parent: null, position: 2 },
      ],
      "knowledge.article": [
        {
          id: vpnArticle,
          title: "VPN verbindet nicht",
          body: "<p>Starten Sie den VPN-Client neu.</p>",
          category: vpn,
          updated_at: "2026-10-28T09:00:00Z",
        },
        {
          id: printerArticle,
          title: "Drucker im Flur",
          body: "<p>Den Drucker hinzufügen.</p>",
          category: printer,
          updated_at: "2026-10-27T09:00:00Z",
        },
      ],
      "demo.ticket": [{ id: ticket, title: "Kein Netz", category: vpn, version: 1 }],
    };
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("browses by the categories and searches", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/portal");
    const areas = await screen.findByRole("navigation", { name: "Bereiche" });
    expect(within(areas).getByRole("link", { name: "Meine Vorgänge" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    // Only the type to create has a card; the one to read has its area.
    expect(await screen.findByRole("region", { name: "Meldung" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Artikel" })).toBeNull();

    await user.click(within(areas).getByRole("link", { name: "Wissen" }));
    expect(await screen.findByRole("heading", { name: "Wissen" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "VPN verbindet nicht" })).toBeInTheDocument();
    expect(screen.getByText("Starten Sie den VPN-Client neu.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Drucker im Flur" })).toBeInTheDocument();

    // A category narrows to it and what lies below: VPN under Netz und Telefonie.
    const tree = screen.getByRole("navigation", { name: "Kategorie" });
    await user.click(within(tree).getByRole("link", { name: "Netz und Telefonie" }));
    expect(
      await screen.findByRole("heading", { name: "Wissen: Netz und Telefonie" }),
    ).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("link", { name: "Drucker im Flur" })).toBeNull());
    expect(screen.getByRole("link", { name: "VPN verbindet nicht" })).toBeInTheDocument();
    expect(within(tree).getByRole("link", { name: "Netz und Telefonie" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(tree).getByRole("link", { name: "Alle" })).not.toHaveAttribute("aria-current");

    // The search by the words.
    await user.type(screen.getByLabelText("Wissen durchsuchen"), "Drucker{Enter}");
    expect(
      await screen.findByRole("heading", { name: "Treffer für „Drucker“" }),
    ).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "Drucker im Flur" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "VPN verbindet nicht" })).toBeNull();
    await expectAccessible(container);
  });

  it("shows an article with its category by name", async () => {
    const { container } = renderAt(`/portal/read/knowledge.article/${vpnArticle}`);
    expect(await screen.findByRole("heading", { name: "VPN verbindet nicht" })).toBeInTheDocument();
    expect(await screen.findByText("VPN", { selector: "span" })).toBeInTheDocument();
    expect(screen.getByText("Starten Sie den VPN-Client neu.")).toBeInTheDocument();
    const crumbs = screen.getByRole("navigation", { name: "breadcrumb" });
    expect(within(crumbs).getByRole("link", { name: "Wissen" })).toHaveAttribute(
      "href",
      `/portal/read/knowledge.article?group=${vpn}`,
    );
    expect(screen.getByRole("link", { name: "Meldung anlegen" })).toHaveAttribute(
      "href",
      "/portal/demo.ticket/new",
    );
    await expectAccessible(container);
  });

  // SD16c: shown records come in the person's language, the form reads German.
  it("reads in the language of the person, the form in German", async () => {
    initI18n("en");
    const article = renderAt(`/portal/read/knowledge.article/${vpnArticle}`);
    expect(await screen.findByRole("heading", { name: "VPN verbindet nicht" })).toBeInTheDocument();
    const read = state.calls.find((c) => c.method === "GET" && c.path.endsWith(vpnArticle!));
    expect(read?.lang).toBe("en");
    article.unmount();
    state.workplace = true;
    renderAt(`/types/probe.probe/${state.records[0]!.id}`);
    expect(await screen.findByRole("heading", { name: "Probe 000001" })).toBeInTheDocument();
    const edit = state.calls.find(
      (c) => c.method === "GET" && c.path === `/api/v1/records/probe.probe/${state.records[0]!.id}`,
    );
    expect(edit?.lang).toBe("de");
  });

  it("shows a reference of one's own record by name", async () => {
    renderAt(`/portal/demo.ticket/${ticket}`);
    expect(await screen.findByRole("heading", { name: "Kein Netz" })).toBeInTheDocument();
    expect(await screen.findByText("VPN")).toBeInTheDocument();
    expect(screen.queryByText(vpn!)).toBeNull();
  });
});
