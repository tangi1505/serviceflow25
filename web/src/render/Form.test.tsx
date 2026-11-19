// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The form from the definition: one control per field type, rules from the fields, create and
// edit through the fake API, and the conflict view when the version moved on (E29).
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, probeType, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

import { schemaFor, toInput, toRecord } from "./fields";
import { selectOption } from "@/test/select";

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

describe("fields", () => {
  it("turn the definition into zod rules", () => {
    const schema = schemaFor(probeType);
    expect(schema.safeParse({ title: "", priority: 3, active: true, kind: "fault" }).success).toBe(
      false,
    );
    expect(schema.safeParse({ title: "x", priority: 9, active: true, kind: "fault" }).success).toBe(
      false,
    );
    expect(schema.safeParse({ title: "x", priority: 2, active: true, kind: "nope" }).success).toBe(
      false,
    );
    expect(
      schema.safeParse({ title: "x", priority: 2, active: true, kind: "fault", parent: "nope" })
        .success,
    ).toBe(false);
    const ok = schema.safeParse({
      title: "x",
      priority: 2,
      active: false,
      kind: "wish",
      due_on: "",
      seen_at: "",
      parent: "",
    });
    expect(ok.success).toBe(true);
  });

  it("never send calculated fields (PL20, PL22)", () => {
    const withSums = {
      ...probeType,
      fields: [
        ...probeType.fields,
        { name: "cost_total", type: "number" as const, trait: "costs", computed: true },
        { name: "score", type: "number" as const, formula: "priority * 2.0" },
      ],
    };
    const body = toRecord(withSums, {
      ...toInput(withSums, undefined),
      title: "t",
      cost_total: 5,
      score: 4,
    });
    expect(body).not.toHaveProperty("cost_total");
    expect(body).not.toHaveProperty("score");
    expect(
      schemaFor(withSums).safeParse({
        title: "x",
        priority: 2,
        active: true,
        kind: "wish",
        cost_total: "x",
      }).success,
    ).toBe(true);
  });

  it("map defaults into the form and empty input to null", () => {
    const input = toInput(probeType, undefined);
    expect(input).toMatchObject({
      title: "",
      priority: 3,
      active: true,
      kind: "question",
      parent: "",
    });
    const body = toRecord(probeType, {
      ...input,
      title: "t",
      seen_at: "2026-10-20T10:30",
      priority: Number.NaN,
    });
    expect(body["parent"]).toBeNull();
    expect(body["priority"]).toBeNull();
    expect(body["seen_at"]).toMatch(/^2026-10-20T/);
  });
});

describe("Form", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(2);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("creates a record with one control per field type", async () => {
    const user = userEvent.setup();
    const { router, container } = renderAt("/types/probe.probe/new");
    await screen.findByRole("heading", { name: "Neuer Datensatz" });
    expect(screen.getByLabelText(/Titel/)).toHaveAttribute("aria-required", "true");
    expect(screen.getByLabelText("Priorität")).toHaveAttribute("type", "number");
    expect(screen.getByLabelText("Aktiv")).toBeChecked();
    expect(screen.getByLabelText("Fällig am")).toHaveAttribute("type", "date");
    expect(screen.getByLabelText("Gesehen")).toHaveAttribute("type", "datetime-local");
    expect(screen.getByLabelText("Art")).toHaveAttribute("data-value", "question");
    await expectAccessible(container);

    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    expect(await screen.findByText("Bitte ausfüllen.")).toBeInTheDocument();

    await user.type(screen.getByLabelText(/Titel/), "Drucker streikt");
    await selectOption(user, screen.getByLabelText("Art"), "fault");
    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    await waitFor(() =>
      expect(router.state.location.pathname).toMatch(/^\/types\/probe\.probe\/01a15a3c/),
    );
    expect(state.records[2]).toMatchObject({
      title: "Drucker streikt",
      kind: "fault",
      priority: 3,
    });
    await screen.findByRole("heading", { name: "Drucker streikt" });
  });

  it("edits with If-Match and shows both values on a conflict", async () => {
    const user = userEvent.setup();
    const first = state.records[0]!;
    const { router } = renderAt(`/types/probe.probe/${first["id"]}`);
    const title = await screen.findByLabelText(/Titel/);
    expect(title).toHaveValue("Probe 000001");

    // Someone else saves in between: the server's version moves to 2 with another title.
    first["version"] = 2;
    first["title"] = "Von Kollegin geändert";
    await user.clear(title);
    await user.type(title, "Meine Fassung");
    await user.click(screen.getByRole("button", { name: "Speichern" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Jemand hat diesen Datensatz inzwischen gespeichert.");
    expect(alert).toHaveTextContent("Von Kollegin geändert");
    expect(alert).toHaveTextContent("Meine Fassung");
    const patches = state.calls.filter((c) => c.method === "PATCH");
    expect(patches).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: "Meine Eingabe trotzdem speichern" }));
    // Saving stays on the record page (UI3, Pascal's decision 2026-10-29; until then it went
    // back to the list).
    expect(await screen.findByText("Gespeichert.")).toBeInTheDocument();
    expect(screen.getByText("Version 3")).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(`/types/probe.probe/${first["id"]}`);
    expect(first).toMatchObject({ title: "Meine Fassung", version: 3 });
  });

  it("follows the field states of a new record: hidden, required, read-only, preset (WF2)", async () => {
    const user = userEvent.setup();
    state.newStates = {
      hidden: ["seen_at"],
      required: ["due_on"],
      read_only: ["parent"],
      defaults: { priority: 4 },
    };
    const { container } = renderAt("/types/probe.probe/new");
    await screen.findByRole("heading", { name: "Neuer Datensatz" });
    expect(screen.queryByLabelText("Gesehen")).not.toBeInTheDocument();
    expect(screen.getByLabelText(/Fällig am/)).toHaveAttribute("aria-required", "true");
    expect(screen.getByLabelText("Übergeordnet")).toHaveAttribute("readonly");
    expect(screen.getByLabelText("Priorität")).toHaveValue(4);
    await expectAccessible(container);

    await user.type(screen.getByLabelText(/Titel/), "Wunsch");
    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    expect(await screen.findByText("Bitte ausfüllen.")).toBeInTheDocument();
    await user.type(screen.getByLabelText(/Fällig am/), "2026-11-02");
    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    await waitFor(() => expect(state.records).toHaveLength(3));
    // The read-only field is not sent; the hidden one neither.
    expect(state.records[2]).toMatchObject({ priority: 4, due_on: "2026-11-02", parent: null });
    expect(state.records[2]).not.toHaveProperty("seen_at", "");
  });

  it("lets the person choose the node of a new record when several are allowed (PL46)", async () => {
    const user = userEvent.setup();
    state.newStates = {
      node: "",
      nodes: [
        { path: "root.nord", label: { de: "Polizei Nord" } },
        { path: "root.sued", label: { de: "Polizei Süd" } },
      ],
    };
    const { container } = renderAt("/types/probe.probe/new");
    await screen.findByRole("heading", { name: "Neuer Datensatz" });
    const node = screen.getByLabelText(/Ablegen in/);
    expect(node).toHaveValue("");
    await expectAccessible(container);
    await user.type(screen.getByLabelText(/Titel/), "Wo");
    await selectOption(user, node, "root.sued");
    await user.click(screen.getByRole("button", { name: "Anlegen" }));
    await waitFor(() => expect(state.records).toHaveLength(3));
    expect(state.records[2]).toMatchObject({ org_path: "root.sued" });
  });

  it("shows a closed record read-only, without saving (WF2)", async () => {
    const closed = state.records[1]!;
    closed["state"] = "closed";
    const { container } = renderAt(`/types/probe.probe/${closed["id"]}`);
    expect(await screen.findByText(/im Status „closed“ schreibgeschützt/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Titel/)).toHaveAttribute("readonly");
    expect(screen.getByLabelText(/Titel/)).toHaveValue(String(closed["title"]));
    expect(screen.queryByRole("button", { name: "Speichern" })).not.toBeInTheDocument();
    await expectAccessible(container);
  });

  it("leaves out a field the customer layer hid, in the form and the list (PL43)", async () => {
    const fields = probeType.fields;
    probeType.fields = fields.map((f) => (f.name === "due_on" ? { ...f, hidden: true } : f));
    try {
      const create = renderAt("/types/probe.probe/new");
      await screen.findByLabelText(/Titel/);
      expect(screen.queryByLabelText("Fällig am")).toBeNull();
      create.unmount();
      renderAt("/types/probe.probe");
      await screen.findByRole("columnheader", { name: /Titel/ });
      expect(screen.queryByRole("columnheader", { name: "Fällig am" })).toBeNull();
    } finally {
      probeType.fields = fields;
    }
  });
});
