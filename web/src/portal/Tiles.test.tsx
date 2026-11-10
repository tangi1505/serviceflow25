// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The start page of the portal (SD14, SD-Plan E13): the tiles in their order, each to its
// goal, those the person cannot reach left out; they take the place of the card of the forms;
// a notice written by hand stands in the band "Aktuell" with its title and until when.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FormOffer } from "@/api/client";
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

const id = (n: number) => `01a15a3c-8545-7691-b780-d000000000${String(n).padStart(2, "0")}`;
const form = id(90);
const article = id(91);

const tile = (n: number, values: Record<string, unknown>) => ({
  id: id(n),
  version: 1,
  position: n * 10,
  description: "",
  ...values,
});

describe("Start page of the portal", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.workplace = false;
    state.extraTypes = [articleType];
    state.forms = [
      {
        id: form,
        name: "Zugang beantragen",
        target_type: "probe.probe",
        definition: { sections: [] },
      } as unknown as FormOffer,
    ];
    state.extraRecords = {
      "core.portal_tile": [
        tile(1, {
          name: "Probe melden",
          description: "Etwas geht nicht?",
          kind: "create",
          target_type: "probe.probe",
        }),
        tile(2, { name: "Zugang beantragen", kind: "form", form }),
        tile(3, { name: "Altes Formular", kind: "form", form: id(99) }),
        tile(4, { name: "Wissen", kind: "area", target_type: "knowledge.article" }),
        tile(5, {
          name: "VPN im Homeoffice",
          kind: "record",
          target_type: "knowledge.article",
          target_id: article,
        }),
        tile(6, { name: "Intranet", kind: "link", url: "https://intranet.example/" }),
        tile(7, { name: "Böse", kind: "link", url: "javascript:alert(1)" }),
        tile(8, { name: "Unbekannt", kind: "area", target_type: "cmdb.server" }),
      ],
    };
    state.notices = [
      {
        type_name: "core.announcement",
        id: id(80),
        title: "Wartung VPN am Samstag",
        text: "<p>Kein Zugang von außen.</p>",
        format: "html",
        since: "2026-11-12T07:00:00Z",
        until: "2026-11-15T11:00:00Z",
        updated_at: "2026-11-10T09:00:00Z",
      },
    ];
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the tiles the person can reach, in their order", async () => {
    const { container } = renderAt("/portal");
    const section = await screen.findByRole("region", { name: "Was möchten Sie tun?" });
    const links = within(section).getAllByRole("link");
    expect(links.map((l) => l.querySelector("b")?.textContent)).toEqual([
      "Probe melden",
      "Zugang beantragen",
      "Wissen",
      "VPN im Homeoffice",
      "Intranet",
    ]);
    expect(links[0]).toHaveAttribute("href", "/portal/probe.probe/new");
    expect(links[0]).toHaveTextContent("ProbeProbe meldenEtwas geht nicht?");
    expect(links[1]).toHaveAttribute("href", `/portal/forms/${form}`);
    expect(links[2]).toHaveAttribute("href", "/portal/read/knowledge.article");
    expect(links[3]).toHaveAttribute("href", `/portal/read/knowledge.article/${article}`);
    expect(links[4]).toHaveAttribute("href", "https://intranet.example/");
    expect(links[4]).toHaveAttribute("target", "_blank");
    expect(links[4]).toHaveAttribute("rel", "noopener noreferrer");
    expect(links[4]).toHaveAccessibleName(/öffnet in neuem Fenster/);
    // The tiles take the place of the card of the forms.
    expect(screen.queryByRole("region", { name: "Formulare" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Meine Vorgänge", level: 2 })).toBeInTheDocument();
    await expectAccessible(container);
  });

  it("keeps the forms for a person without tiles", async () => {
    state.extraRecords = { "core.portal_tile": [] };
    renderAt("/portal");
    expect(await screen.findByRole("region", { name: "Formulare" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Was möchten Sie tun?" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Meine Vorgänge", level: 1 })).toBeInTheDocument();
  });

  it("shows a notice written by hand with its title and until when", async () => {
    const user = userEvent.setup();
    renderAt("/portal");
    const band = await screen.findByRole("region", { name: "Aktuell" });
    expect(within(band).getByText("Wartung VPN am Samstag")).toBeInTheDocument();
    expect(band).toHaveTextContent(/bis 15\.11\.2026/);
    expect(within(band).queryByText("Kein Zugang von außen.")).toBeNull();
    await user.click(within(band).getByRole("button", { name: "Mehr" }));
    expect(within(band).getByText("Kein Zugang von außen.")).toBeInTheDocument();
  });
});
