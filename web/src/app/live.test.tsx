// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Live (UI8) with a stand-in for EventSource: the record page shows who else is on it and
// follows a change by someone else, or says so when its form holds changes; the list takes a
// changed row in and shows new records as a note; a notification comes for a record of mine
// changed by someone else while the tab is out of view.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, me, newState, type FakeState } from "@/test/fakeApi";

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

// FakeSource stands in for the browser's EventSource.
class FakeSource {
  static last: FakeSource | null = null;
  listeners = new Map<string, ((m: MessageEvent<string>) => void)[]>();
  constructor(public url: string) {
    FakeSource.last = this;
  }
  addEventListener(kind: string, fn: (m: MessageEvent<string>) => void) {
    this.listeners.set(kind, [...(this.listeners.get(kind) ?? []), fn]);
  }
  close() {}
  emit(kind: string, data: Record<string, unknown>) {
    act(() => {
      for (const fn of this.listeners.get(kind) ?? []) {
        fn(new MessageEvent(kind, { data: JSON.stringify({ kind, ...data }) }));
      }
    });
  }
}

const anna = "01a15a3c-8545-7691-b780-a00000000001";

describe("Live", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(3);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
    vi.stubGlobal("EventSource", FakeSource);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    FakeSource.last = null;
  });

  it("shows who is on the record and follows changes by others", async () => {
    const user = userEvent.setup();
    const rec = state.records[0]!;
    const id = String(rec["id"]);
    state.others = [{ account: anna, name: "Anna", editing: true, since: "", at: "" }];
    renderAt(`/types/probe.probe/${id}`);
    await screen.findByRole("heading", { name: "Probe 000001" });
    expect(await screen.findByText("Anna bearbeitet gerade")).toBeInTheDocument();
    expect(FakeSource.last?.url).toBe("/api/v1/events/stream");

    // Anna saves while the form holds nothing: the page follows.
    Object.assign(rec, { title: "Von Anna", version: 2 });
    FakeSource.last!.emit("update", { type_name: "probe.probe", record_id: id, actor: anna });
    expect(await screen.findByRole("heading", { name: "Von Anna" })).toBeInTheDocument();

    // Now with a change in the form: the form stays, a note says who changed it.
    await user.type(screen.getByLabelText(/Titel/), " und ich");
    Object.assign(rec, { title: "Wieder Anna", version: 3 });
    FakeSource.last!.emit("update", { type_name: "probe.probe", record_id: id, actor: anna });
    expect(await screen.findByText(/Geändert von Anna/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Titel/)).toHaveValue("Von Anna und ich");
    await user.click(screen.getByRole("button", { name: "Neu laden" }));
    expect(await screen.findByRole("heading", { name: "Wieder Anna" })).toBeInTheDocument();
    expect(screen.queryByText(/Geändert von Anna/)).toBeNull();

    // My own changes from another window do not count.
    Object.assign(rec, { title: "Ich selbst", version: 4 });
    FakeSource.last!.emit("update", { type_name: "probe.probe", record_id: id, actor: me.id });
    await new Promise((r) => setTimeout(r, 100));
    expect(screen.getByRole("heading", { name: "Wieder Anna" })).toBeInTheDocument();
  });

  it("takes a changed row into the list and notes new records", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe");
    await screen.findByText("3 Datensätze");
    const rec = state.records[1]!;
    Object.assign(rec, { title: "Neu benannt", version: 2 });
    FakeSource.last!.emit("update", {
      type_name: "probe.probe",
      record_id: rec["id"],
      actor: anna,
    });
    expect(await screen.findByText("Neu benannt", {}, { timeout: 4000 })).toBeInTheDocument();
    const batched = state.calls.filter((c) => c.path === "/api/v1/records/probe.probe:query");
    expect(batched).toHaveLength(1);
    expect(JSON.parse(batched[0]!.body!)).toMatchObject({
      where: { field: "id", in: [rec["id"]] },
    });

    state.records.push({
      ...rec,
      id: "01a15a3c-8545-7691-b780-000000000099",
      title: "Ganz neu",
      version: 1,
    });
    FakeSource.last!.emit("create", { type_name: "probe.probe", record_id: "x", actor: anna });
    expect(screen.queryByText("Ganz neu")).toBeNull();
    await user.click(await screen.findByRole("button", { name: "1 neuer Datensatz – anzeigen" }));
    expect(await screen.findByText("Ganz neu")).toBeInTheDocument();
  });

  it("notifies of my records changed by others while the tab is out of view", async () => {
    const shown: { title: string; body?: string }[] = [];
    class FakeNotification {
      static permission = "default";
      static requestPermission = vi.fn(async () => {
        FakeNotification.permission = "granted";
        return "granted";
      });
      onclick: (() => void) | null = null;
      constructor(title: string, opts?: { body?: string }) {
        shown.push({ title, body: opts?.body });
      }
      close() {}
    }
    vi.stubGlobal("Notification", FakeNotification);
    const hidden = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    const user = userEvent.setup();
    renderAt("/types/probe.probe");
    await screen.findByText("3 Datensätze");
    // Since UI35 the window of the bell asks for the permission.
    const header = screen.getByRole("banner");
    await user.click(within(header).getByRole("button", { name: /^Benachrichtigungen/ }));
    await user.click(within(header).getByRole("button", { name: "Benachrichtigungen erlauben" }));
    expect(
      within(header).queryByRole("button", { name: "Benachrichtigungen erlauben" }),
    ).toBeNull();
    await user.keyboard("{Escape}");

    const id = String(state.records[0]!["id"]);
    FakeSource.last!.emit("comment", {
      type_name: "probe.probe",
      record_id: id,
      actor: anna,
      mine: true,
    });
    FakeSource.last!.emit("update", { type_name: "probe.probe", record_id: id, actor: anna });
    FakeSource.last!.emit("update", {
      type_name: "probe.probe",
      record_id: id,
      actor: me.id,
      mine: true,
    });
    expect(shown).toEqual([{ title: "Probe", body: "Neuer Eintrag" }]);
    hidden.mockRestore();
  });
});
