// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The workflow editor as a table (UI26.1, E30): by keyboard alone a state and a transition
// with two from states and a condition; the condition is checked while typing and an invalid
// one blocks the row; a state no transition reaches is pointed out; disabling a transition of
// the package and taking a change back are one change each. The graph (UI26.2, E31):
// an edge marks its transition in graph and table, a moved node and "Anordnen" make the
// layout unsaved, "Anordnung speichern" is one change with every state, and the graph
// folds and remembers it.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";
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

function sent(state: FakeState) {
  return state.calls
    .filter((c) => c.method === "POST" && c.path.endsWith("/customer"))
    .map((c) => (JSON.parse(c.body ?? "{}") as { change: Record<string, unknown> }).change);
}

// React Flow measures nodes and edges with layout APIs jsdom lacks; these stand in for them
// (the advice of the library for tests).
const sizes = {
  offsetHeight: Object.getOwnPropertyDescriptor(HTMLElement.prototype, "offsetHeight"),
  offsetWidth: Object.getOwnPropertyDescriptor(HTMLElement.prototype, "offsetWidth"),
};
beforeAll(() => {
  Object.defineProperties(HTMLElement.prototype, {
    offsetHeight: {
      configurable: true,
      get() {
        return parseFloat((this as HTMLElement).style.height) || 1;
      },
    },
    offsetWidth: {
      configurable: true,
      get() {
        return parseFloat((this as HTMLElement).style.width) || 1;
      },
    },
  });
  (SVGElement.prototype as unknown as { getBBox: () => DOMRect }).getBBox = () =>
    ({ x: 0, y: 0, width: 0, height: 0 }) as DOMRect;
});
afterAll(() => {
  Object.defineProperties(HTMLElement.prototype, {
    offsetHeight: sizes.offsetHeight!,
    offsetWidth: sizes.offsetWidth!,
  });
});

function stubLayout() {
  // The observer reports once right away, so that React Flow measures the node and its
  // handles (edges need the handles).
  class ResizeObserver {
    cb: (entries: { target: Element; contentRect: DOMRect }[], o: ResizeObserver) => void;
    constructor(
      cb: (entries: { target: Element; contentRect: DOMRect }[], o: ResizeObserver) => void,
    ) {
      this.cb = cb;
    }
    seen = new Set<Element>();
    observe(el: Element) {
      this.seen.add(el);
      queueMicrotask(() => {
        if (this.seen.has(el) && el.isConnected) {
          this.cb([{ target: el, contentRect: el.getBoundingClientRect() }], this);
        }
      });
    }
    unobserve(el: Element) {
      this.seen.delete(el);
    }
    disconnect() {
      this.seen.clear();
    }
  }
  class DOMMatrixReadOnly {
    m22: number;
    constructor(transform?: string) {
      const scale = transform?.match(/scale\(([\d.]+)\)/)?.[1];
      this.m22 = scale ? Number(scale) : 1;
    }
  }
  vi.stubGlobal("ResizeObserver", ResizeObserver);
  vi.stubGlobal("DOMMatrixReadOnly", DOMMatrixReadOnly);
}

// Like the form builder, long keyboard tests: 15 s instead of 5 under load (PL52).
describe("Workflow editor", { timeout: 15_000 }, () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.customize = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
    stubLayout();
    localStorage.clear();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("adds a state and a transition with two from states and a condition, by keyboard", async () => {
    const user = userEvent.setup();
    const view = renderAt("/types/probe.probe");
    await screen.findByRole("heading", { name: "Probe" });
    await user.click(screen.getByRole("button", { name: "Anpassen" }));
    await user.click(screen.getByRole("menuitem", { name: /Workflow/ }));
    await screen.findByRole("heading", { name: "Workflow: Probe" });
    await expectAccessible(view.container);

    // The state, typed into the form and submitted with Enter.
    const states = screen.getByRole("region", { name: "Status" });
    await user.type(within(states).getByLabelText("Beschriftung"), "Prüfung");
    expect(within(states).getByLabelText("Technischer Name")).toHaveValue("k_pruefung");
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Status „Prüfung“ angelegt.")).toBeInTheDocument();
    // Nothing leads there yet.
    expect(screen.getByText("Kein Übergang führt nach „Prüfung“.")).toBeInTheDocument();

    // The transition: from Neu and In Arbeit to Prüfung, when the priority is low.
    const transitions = screen.getByRole("region", { name: "Übergänge" });
    const form = within(transitions).getByRole("form", { name: "Neuer Übergang" });
    await user.type(within(form).getByLabelText("Beschriftung"), "Prüfen");
    await user.click(within(form).getByRole("checkbox", { name: "Neu" }));
    await user.click(within(form).getByRole("checkbox", { name: "In Arbeit" }));
    await selectOption(user, within(form).getByLabelText("Nach"), "k_pruefung");
    const when = within(form).getByLabelText("Bedingung");
    await user.type(when, "record.priority <");
    expect(await within(form).findByText(/Bedingung ungültig: Syntax error/)).toBeInTheDocument();
    expect(within(form).getByRole("button", { name: "Übergang anlegen" })).toBeDisabled();
    await user.type(when, "= 2");
    expect(await within(form).findByText("Bedingung ist gültig.")).toBeInTheDocument();
    await user.click(within(form).getByRole("button", { name: "Übergang anlegen" }));
    expect(await screen.findByText("Übergang „Prüfen“ angelegt.")).toBeInTheDocument();
    expect(screen.queryByText("Kein Übergang führt nach „Prüfung“.")).toBeNull();

    expect(sent(state)).toEqual([
      {
        op: "add_state",
        field: "k_pruefung",
        state: { name: "k_pruefung", label: { de: "Prüfung" } },
      },
      {
        op: "add_transition",
        field: "k_pruefen",
        transition: {
          name: "k_pruefen",
          label: { de: "Prüfen" },
          from: ["new", "in_progress"],
          to: "k_pruefung",
          when: "record.priority <= 2",
        },
      },
    ]);
    // The new row stands in the table with its states.
    expect(
      within(transitions).getByRole("row", { name: /Prüfen.*Neu, In Arbeit.*Prüfung/ }),
    ).toBeInTheDocument();
    await expectAccessible(view.container);
  });

  it("disables a transition of the package, moves the initial state and takes back", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe/workflow");
    await screen.findByRole("heading", { name: "Workflow: Probe" });
    const transitions = screen.getByRole("region", { name: "Übergänge" });
    await user.click(within(transitions).getByRole("button", { name: "„Erledigen“ stilllegen" }));
    expect(await screen.findByText("Übergang „Erledigen“ stillgelegt.")).toBeInTheDocument();
    expect(screen.getByText("Der Übergang „Erledigen“ ist stillgelegt.")).toBeInTheDocument();
    expect(
      within(transitions).getByRole("button", { name: "„Erledigen“ aktivieren" }),
    ).toBeInTheDocument();
    // A transition of the package has no "remove"; an own one would.
    expect(within(transitions).queryByRole("button", { name: "„Erledigen“ entfernen" })).toBeNull();

    const states = screen.getByRole("region", { name: "Status" });
    await user.click(within(states).getByRole("radio", { name: "Anfangsstatus: In Arbeit" }));
    expect(await screen.findByText("„In Arbeit“ ist jetzt der Anfangsstatus.")).toBeInTheDocument();
    expect(within(states).getByRole("radio", { name: "Anfangsstatus: In Arbeit" })).toBeChecked();
    expect(within(states).getByRole("radio", { name: "Anfangsstatus: Neu" })).not.toBeChecked();

    expect(sent(state)).toEqual([
      { op: "disable_transition", field: "finish" },
      { op: "change_state", field: "in_progress", set: { initial: true } },
    ]);

    // The changes of the workflow, newest first, each to take back.
    const changes = screen.getByRole("region", { name: "Änderungen" });
    await user.click(
      within(changes).getByRole("button", {
        name: "Zurücknehmen: Übergang „Erledigen“ stillgelegt",
      }),
    );
    expect(
      await screen.findByText("Zurückgenommen: Übergang „Erledigen“ stillgelegt."),
    ).toBeInTheDocument();
    expect(
      within(transitions).getByRole("button", { name: "„Erledigen“ stilllegen" }),
    ).toBeInTheDocument();
  });

  it("renames a state and reroutes a transition of the package", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe/workflow");
    await screen.findByRole("heading", { name: "Workflow: Probe" });
    const states = screen.getByRole("region", { name: "Status" });
    await user.click(within(states).getByRole("button", { name: "„Erledigt“ bearbeiten" }));
    const rowForm = within(states).getByRole("form", { name: "„Erledigt“ bearbeiten" });
    const row = within(rowForm).getByLabelText("Beschriftung");
    await user.clear(row);
    await user.type(row, "Fertig");
    await user.click(within(rowForm).getByRole("button", { name: "Speichern" }));
    expect(await screen.findByText("Status „Fertig“ geändert.")).toBeInTheDocument();

    const transitions = screen.getByRole("region", { name: "Übergänge" });
    await user.click(within(transitions).getByRole("button", { name: "„Beginnen“ bearbeiten" }));
    const editor = within(transitions).getByRole("form", { name: "„Beginnen“ bearbeiten" });
    await user.click(within(editor).getByRole("checkbox", { name: "Fertig" }));
    await user.click(within(editor).getByRole("button", { name: "Speichern" }));
    expect(await screen.findByText("Übergang „Beginnen“ geändert.")).toBeInTheDocument();
    expect(sent(state)).toEqual([
      { op: "change_state", field: "done", set: { label: { de: "Fertig" } } },
      { op: "change_transition", field: "start", set: { from: ["new", "done"] } },
    ]);
  });

  it("marks a transition from its edge, arranges and saves the layout as one change", async () => {
    const user = userEvent.setup();
    const view = renderAt("/types/probe.probe/workflow");
    await screen.findByRole("heading", { name: "Workflow: Probe" });
    const graph = screen.getByRole("region", { name: "Graph" });
    // Every state is a node, every from state of a transition an edge.
    for (const name of ["Neu", "In Arbeit", "Erledigt"]) {
      expect(within(graph).getByText(name)).toBeInTheDocument();
    }
    const edge = await within(graph).findByLabelText(
      "Übergang „Erledigen“ von „In Arbeit“ nach „Erledigt“",
    );
    await user.click(edge);
    const row = screen.getByRole("row", { name: /Erledigen/ });
    expect(row).toHaveAttribute("aria-selected", "true");
    expect(within(row).getByText(/im Graphen markiert/)).toBeInTheDocument();
    expect(screen.getByRole("row", { name: /Beginnen/ })).toHaveAttribute("aria-selected", "false");
    // The row picks the transition too.
    await user.click(
      within(screen.getByRole("row", { name: /Wieder öffnen/ })).getByText("Wieder öffnen"),
    );
    expect(screen.getByRole("row", { name: /Wieder öffnen/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("row", { name: /Erledigen/ })).toHaveAttribute(
      "aria-selected",
      "false",
    );

    // Nothing to save until something moved; "Anordnen" moves, saving is one change with
    // the position of every state.
    expect(screen.getByRole("button", { name: "Anordnung speichern" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Anordnen" }));
    expect(screen.getByText("Anordnung verändert, noch nicht gespeichert.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Anordnung speichern" }));
    expect(await screen.findByText("Anordnung des Graphen gespeichert.")).toBeInTheDocument();
    const changes = sent(state);
    expect(changes).toHaveLength(1);
    expect(changes[0]!.op).toBe("set_layout");
    expect(Object.keys(changes[0]!.layout as object).sort()).toEqual([
      "done",
      "in_progress",
      "new",
    ]);
    expect(screen.queryByText("Anordnung verändert, noch nicht gespeichert.")).toBeNull();
    expect(
      within(screen.getByRole("region", { name: "Änderungen" })).getByText(
        /Anordnung des Graphen gespeichert/,
      ),
    ).toBeInTheDocument();
    await expectAccessible(view.container);
  });

  it("folds the graph and remembers it", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe/workflow");
    await screen.findByRole("heading", { name: "Workflow: Probe" });
    expect(screen.getByTestId("workflow-graph")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Graph einklappen" }));
    expect(screen.queryByTestId("workflow-graph")).toBeNull();
    expect(screen.queryByRole("button", { name: "Anordnen" })).toBeNull();
    expect(localStorage.getItem("itsm.workflow.graph.01a15a3c-8545-7691-b780-9b915e555b1c")).toBe(
      "closed",
    );
    cleanup();
    renderAt("/types/probe.probe/workflow");
    await screen.findByRole("heading", { name: "Workflow: Probe" });
    expect(screen.queryByTestId("workflow-graph")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Graph aufklappen" }));
    expect(screen.getByTestId("workflow-graph")).toBeInTheDocument();
  });
});
