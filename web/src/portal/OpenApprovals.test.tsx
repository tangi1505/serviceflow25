// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// "Wartet auf Ihre Genehmigung" on the start page of the portal (UI44, UI plan E43): the open
// questions with "Zustimmen" and "Ablehnen" as on the page of the approval; a decision takes
// the row away and lowers the number at the area; without a question no section.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Approval, MyApproval } from "@/api/client";
import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { approverPerson, fakeApi, newState, type FakeState } from "@/test/fakeApi";
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

const record = (n: number) => `01a15a3c-8545-7691-b780-e000000000${n}0`;
const approvalId = (n: number) => `0190a000-0000-7000-8000-00000000000${n}`;
const stepId = (n: number) => `0190a000-0000-7000-9000-00000000000${n}`;

// asked is one open question to the person: the approval at the record and its line in "mine".
function asked(n: number, title: string): { approval: Approval; mine: MyApproval } {
  const step = {
    id: stepId(n),
    approver: { type: "core.person" as const, id: approverPerson },
    state: "open" as const,
    mine: true,
  };
  return {
    approval: {
      id: approvalId(n),
      state: "open",
      stage: 1,
      message: "",
      requester: approverPerson,
      requested_at: "2026-11-16T08:00:00Z",
      stages: [{ position: 1, name: "", need: 0, state: "open", steps: [step] }],
    } as Approval,
    mine: {
      approval: approvalId(n),
      step: stepId(n),
      type_name: "probe.probe",
      record_id: record(n),
      title,
      stage: 1,
      stages: 1,
      stage_name: "",
      approver: { type: "core.person", id: approverPerson },
      state: "open",
      requested_at: `2026-11-16T0${n}:00:00Z`,
    } as MyApproval,
  };
}

describe("Open approvals on the start page", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(0);
    state.signedIn = true;
    state.workplace = false;
    const a = asked(1, "Neuer Laptop");
    const b = asked(2, "Zugang Laufwerk");
    state.approvals = { [record(1)]: [a.approval], [record(2)]: [b.approval] };
    state.myApprovals = { open: [a.mine, b.mine] };
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows two questions; agreeing takes one away and lowers the number", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/portal");
    const section = await screen.findByRole("region", { name: "Wartet auf Ihre Genehmigung" });
    expect(within(section).getAllByRole("listitem")).toHaveLength(2);
    expect(within(section).getByRole("link", { name: "Neuer Laptop" })).toHaveAttribute(
      "href",
      `/portal/approvals/probe.probe/${record(1)}`,
    );
    const area = screen.getByRole("link", { name: /Genehmigungen/ });
    await waitFor(() => expect(area).toHaveTextContent("2"));
    await expectAccessible(container);

    await user.click(within(section).getByRole("button", { name: "Zustimmen: Neuer Laptop" }));
    await waitFor(() => expect(within(section).getAllByRole("listitem")).toHaveLength(1));
    expect(within(section).queryByRole("link", { name: "Neuer Laptop" })).toBeNull();
    await waitFor(() => expect(area).toHaveTextContent("1"));
    const decided = state.approvals![record(1)]![0]!.stages[0]!.steps[0]!;
    expect(decided.state).toBe("approved");
  });

  it("asks for the reason before refusing, as on the page of the approval", async () => {
    const user = userEvent.setup();
    renderAt("/portal");
    const section = await screen.findByRole("region", { name: "Wartet auf Ihre Genehmigung" });
    await user.click(within(section).getByRole("button", { name: "Ablehnen: Zugang Laufwerk" }));
    const reason = within(section).getByLabelText("Grund der Ablehnung");
    expect(reason).toHaveFocus();
    await user.click(within(section).getByRole("button", { name: "Ablehnen" }));
    expect(within(section).getByRole("alert")).toHaveTextContent(/Grund/);
    expect(state.approvals![record(2)]![0]!.stages[0]!.steps[0]!.state).toBe("open");
    await user.type(reason, "Nicht nötig");
    await user.click(within(section).getByRole("button", { name: "Ablehnen" }));
    await waitFor(() => expect(within(section).getAllByRole("listitem")).toHaveLength(1));
    const refused = state.approvals![record(2)]![0]!.stages[0]!.steps[0]!;
    expect(refused).toMatchObject({ state: "rejected", comment: "Nicht nötig" });
  });

  it("has no section without an open question", async () => {
    state.myApprovals = { open: [] };
    renderAt("/portal");
    await screen.findByRole("region", { name: "Meine Vorgänge" });
    expect(screen.queryByRole("region", { name: "Wartet auf Ihre Genehmigung" })).toBeNull();
  });
});
