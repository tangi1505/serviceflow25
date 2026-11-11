// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The catalog in the portal (SD13, variant 1A of the mockup "Servicekatalog"): "Katalog" is a
// reading area with the areas as a tree and the offers as cards with delivery time and costs;
// the page of an offer shows its form, and ordering leads to the request it made.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FormOffer, TypeDefinition } from "@/api/client";
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

const id = (n: number) => `01a15a3c-8545-7691-b780-c100000000${String(n).padStart(2, "0")}`;
const [access, drives, phones, driveOffer, phoneOffer, form] = [1, 2, 3, 4, 5, 6].map(id);

const areaType: TypeDefinition = {
  name: "area",
  package: "catalog",
  label: { de: "Katalogbereich" },
  title_field: "name",
  traits: ["list"],
  fields: [
    { name: "name", type: "text", label: { de: "Name" } },
    { name: "parent", type: "reference", label: { de: "Übergeordnet" }, to: "catalog.area" },
  ],
};

const offerType: TypeDefinition = {
  name: "offer",
  package: "catalog",
  label: { de: "Angebot" },
  title_field: "name",
  fields: [
    { name: "name", type: "text", label: { de: "Name" } },
    { name: "description", type: "text", format: "rich", label: { de: "Beschreibung" } },
    { name: "area", type: "reference", label: { de: "Katalogbereich" }, to: "catalog.area" },
    { name: "form", type: "reference", label: { de: "Formular" }, to: "core.form" },
    { name: "delivery_days", type: "number", label: { de: "Lieferzeit in Arbeitstagen" } },
    { name: "cost", type: "number", label: { de: "Kosten in Euro" } },
    {
      name: "cost_kind",
      type: "choice",
      label: { de: "Kostenart" },
      options: [{ value: "once", label: { de: "einmalig" } }],
    },
  ],
  portal: {
    read: {
      label: { de: "Katalog", en: "Catalogue" },
      fields: ["description", "delivery_days", "cost", "cost_kind"],
      group: "area",
      facts: ["delivery_days", "cost", "cost_kind"],
      sort: "name",
    },
  },
  order: { form: "form", relation: "catalog.ordered", copy: { title: "name" } },
};

// The request the order makes: a type the portal creates, so its page follows.
const requestType: TypeDefinition = {
  name: "request",
  package: "request",
  label: { de: "Anfrage" },
  title_field: "title",
  fields: [
    { name: "title", type: "text", label: { de: "Titel" } },
    { name: "description", type: "text", label: { de: "Beschreibung" } },
  ],
  portal: { fields: ["title", "description"] },
};

const orderForm: FormOffer = {
  id: form!,
  name: "Zugang Gruppenlaufwerk bestellen",
  target_type: "request.request",
  place: "order",
  definition: {
    sections: [
      {
        id: "zugang",
        title: { de: "Zugang" },
        questions: [
          { id: "laufwerk", type: "text", required: true, label: { de: "Laufwerk" } },
          { id: "warum", field: "description", label: { de: "Begründung" } },
        ],
      },
    ],
  },
};

describe("Catalog in the portal", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.workplace = false;
    state.extraTypes = [areaType, offerType, requestType];
    state.forms = [orderForm];
    state.extraRecords = {
      "catalog.area": [
        { id: access, name: "Zugänge", parent: null, position: 1 },
        { id: drives, name: "Laufwerke", parent: access, position: 1 },
        { id: phones, name: "Telefonie", parent: null, position: 2 },
      ],
      "catalog.offer": [
        {
          id: driveOffer,
          name: "Zugang Gruppenlaufwerk",
          description: "<p>Rechte auf ein Laufwerk Ihrer Dienststelle.</p>",
          area: drives,
          form,
          delivery_days: 3,
          cost_kind: "once",
          updated_at: "2026-11-10T09:00:00Z",
        },
        {
          id: phoneOffer,
          name: "Diensthandy",
          description: "<p>Ein Mobiltelefon mit Vertrag.</p>",
          area: phones,
          form,
          delivery_days: 10,
          updated_at: "2026-11-10T09:00:00Z",
        },
      ],
      "request.request": [],
    };
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists the offers as cards with their facts, sorted by name", async () => {
    const { container } = renderAt("/portal/read/catalog.offer");
    expect(await screen.findByRole("heading", { name: "Katalog" })).toBeInTheDocument();
    const card = (await screen.findByRole("link", { name: "Zugang Gruppenlaufwerk" })).closest(
      "li",
    )!;
    expect(within(card).getByText("Rechte auf ein Laufwerk Ihrer Dienststelle.")).toBeVisible();
    expect(within(card).getByText("Lieferzeit in Arbeitstagen:")).toBeVisible();
    expect(within(card).getByText("3")).toBeVisible();
    expect(within(card).getByText("einmalig")).toBeVisible();
    expect(screen.getByRole("navigation", { name: "Katalogbereich" })).toBeInTheDocument();
    const query = state.calls.find(
      (c) => c.method === "POST" && c.path === "/api/v1/records/catalog.offer:query",
    );
    expect(JSON.parse(query!.body!)).toMatchObject({
      sort: ["name"],
      fields: expect.arrayContaining(["delivery_days", "cost", "cost_kind"]) as unknown,
    });
    await expectAccessible(container);
  });

  it("orders an offer with its form and opens the request", async () => {
    const user = userEvent.setup();
    const { container } = renderAt(`/portal/read/catalog.offer/${driveOffer}`);
    expect(
      await screen.findByRole("heading", { name: "Zugang Gruppenlaufwerk" }),
    ).toBeInTheDocument();
    const order = await screen.findByRole("region", { name: "Bestellen" });
    await expectAccessible(container);
    await user.click(within(order).getByRole("button", { name: "Bestellen" }));
    expect(state.orders ?? []).toHaveLength(0);
    await user.type(within(order).getByLabelText(/Laufwerk/), "N:\\Nord");
    await user.click(within(order).getByRole("button", { name: "Bestellen" }));
    // The page of the request follows, with the offer's name as its title.
    expect(
      await screen.findByRole("heading", { name: "Zugang Gruppenlaufwerk", level: 1 }),
    ).toBeInTheDocument();
    const crumbs = screen.getByRole("navigation", { name: "breadcrumb" });
    expect(within(crumbs).getByRole("link", { name: "Meine Vorgänge" })).toBeInTheDocument();
    expect(state.orders).toEqual([
      { offer: driveOffer, answers: { laufwerk: "N:\\Nord", warum: null } },
    ]);
  });
});
