// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The Prüfweg of UI26.1 in a real browser: in the workflow editor of Probe, by keyboard
// alone, a state and a transition with two from states and a condition; an invalid condition
// is reported at the field; the state is pointed out while nothing leads there; the
// transition is usable over the API at once and the export holds both changes; axe without
// findings. The graph (UI26.2): every state and transition of Probe, a click on an edge
// marks all edges of the transition and its row, a node moved by arrow keys and saved
// stands at the same place after a reload, a dragged connection prefills the form, and
// "Anordnen" arranges anew. The test takes its changes back at the end.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { pick } from "./pick";

const headers = { "X-Requested-With": "itsm" };

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Benutzername").fill("load");
  await page.getByLabel("Kennwort").fill("Messlauf-Pw7!");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}

type Layer = { version: number; changes: { id: string; field: string; op: string }[] };

// mine says whether a change belongs to this test: its names, or the layout of the graph.
const mine = (prefix: string) => (c: { field: string; op: string }) =>
  c.field.startsWith(prefix) || c.op === "set_layout";

// takeBack removes the changes of this test (and of a broken earlier run), newest first.
async function takeBack(page: Page, prefix: string) {
  const layer = (await (
    await page.request.get("/api/v1/types/probe.probe/customer")
  ).json()) as Layer;
  let version = layer.version;
  for (const c of [...layer.changes].reverse().filter(mine(prefix))) {
    const res = await page.request.delete(
      `/api/v1/types/probe.probe/customer/${c.id}?version=${version}`,
      { headers },
    );
    expect(res.status()).toBe(200);
    version = ((await res.json()) as Layer).version;
  }
}

test("a state and a transition made by keyboard work over the API at once", async ({ page }) => {
  const stamp = Date.now();
  const stateLabel = `UI26 Prüfung ${stamp}`;
  const stateName = `k_ui26_pruefung_${stamp}`;
  const trLabel = `UI26 Prüfen ${stamp}`;
  const trName = `k_ui26_pruefen_${stamp}`;
  await signIn(page);
  await takeBack(page, "k_ui26_");
  await page.goto("/types/probe.probe");
  await page.locator("main").getByRole("button", { name: "Anpassen" }).click();
  await page.getByRole("menuitem", { name: /Workflow/ }).click();
  await expect(page.getByRole("heading", { name: "Workflow: Probe" })).toBeVisible();

  // The state.
  const states = page.getByRole("region", { name: "Status" });
  await states.getByLabel("Beschriftung", { exact: true }).focus();
  await page.keyboard.type(stateLabel);
  await expect(states.getByLabel("Technischer Name")).toHaveValue(stateName);
  await page.keyboard.press("Enter");
  await expect(page.getByText(`Status „${stateLabel}“ angelegt.`)).toBeVisible();
  await expect(page.getByText(`Kein Übergang führt nach „${stateLabel}“.`)).toBeVisible();

  // The transition, from Neu and In Arbeit, with a condition typed wrong first.
  const form = page.getByRole("form", { name: "Neuer Übergang" });
  await form.getByLabel("Beschriftung", { exact: true }).focus();
  await page.keyboard.type(trLabel);
  await form.getByRole("checkbox", { name: "Neu" }).focus();
  await page.keyboard.press("Space");
  await form.getByRole("checkbox", { name: "In Arbeit" }).focus();
  await page.keyboard.press("Space");
  await pick(form.getByLabel("Nach"), stateName);
  await form.getByLabel("Bedingung").focus();
  await page.keyboard.type("record.priority <");
  await expect(form.getByText(/Bedingung ungültig/)).toBeVisible();
  await expect(form.getByRole("button", { name: "Übergang anlegen" })).toBeDisabled();
  await page.keyboard.type("= 2");
  await expect(form.getByText("Bedingung ist gültig.")).toBeVisible();
  await form.getByRole("button", { name: "Übergang anlegen" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByText(`Übergang „${trLabel}“ angelegt.`)).toBeVisible();
  await expect(page.getByText(`Kein Übergang führt nach „${stateLabel}“.`)).toHaveCount(0);
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze();
  expect(
    results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`),
  ).toEqual([]);

  // The graph: every state and the new transition's two edges; a click on one edge marks
  // both and the row.
  const graph = page.getByTestId("workflow-graph");
  for (const name of ["Neu", "In Arbeit", "Erledigt", stateLabel]) {
    await expect(graph.getByText(name, { exact: true })).toBeVisible();
  }
  const edgeLabel = `Übergang „${trLabel}“ von „Neu“ nach „${stateLabel}“`;
  // The edge is picked by keyboard: a mouse click would land on the middle of the curve's
  // box, which is not on the curve.
  await graph.locator(`.react-flow__edge[aria-label="${edgeLabel}"]`).focus();
  await page.keyboard.press("Enter");
  // The row in the table of transitions; the list of changes names the transition too.
  const table = page.getByRole("region", { name: "Übergänge" });
  await expect(table.getByRole("row", { name: new RegExp(trLabel) })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(graph.locator(".react-flow__edge.selected")).toHaveCount(2);

  // The new state moves by keyboard; the saved layout survives a reload.
  const node = graph.locator(`.react-flow__node[data-id="${stateName}"]`);
  await node.focus();
  await page.keyboard.press("Enter");
  for (let i = 0; i < 5; i++) {
    await page.keyboard.press("ArrowRight");
  }
  await expect(page.getByText("Anordnung verändert, noch nicht gespeichert.")).toBeVisible();
  // The position is the transform of the node; its z-index changes with the selection.
  const at = (style: string | null) => /transform: ([^;]+);/.exec(style ?? "")?.[1];
  const position = at(await node.getAttribute("style"));
  expect(position).toMatch(/^translate\(/);
  await page.getByRole("button", { name: "Anordnung speichern" }).click();
  await expect(page.getByText("Anordnung des Graphen gespeichert.")).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: "Workflow: Probe" })).toBeVisible();
  await expect
    .poll(async () =>
      at(await graph.locator(`.react-flow__node[data-id="${stateName}"]`).getAttribute("style")),
    )
    .toBe(position);

  // A connection dragged from the new state to Neu prefills the form; the label is typed.
  const backLabel = `UI26 Zurück ${stamp}`;
  const backName = `k_ui26_zurueck_${stamp}`;
  const handle = graph.locator(
    `.react-flow__node[data-id="${stateName}"] .react-flow__handle.source`,
  );
  const from = (await handle.boundingBox())!;
  const to = (await graph.locator('.react-flow__node[data-id="new"]').boundingBox())!;
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 12 });
  await page.mouse.up();
  const back = page.getByRole("form", { name: "Neuer Übergang" });
  await expect(back.getByLabel("Beschriftung", { exact: true })).toBeFocused();
  await expect(back.getByRole("checkbox", { name: stateLabel })).toBeChecked();
  await expect(back.getByLabel("Nach")).toHaveAttribute("data-value", "new");
  await page.keyboard.type(backLabel);
  await expect(back.getByLabel("Technischer Name")).toHaveValue(backName);
  await back.getByRole("button", { name: "Übergang anlegen" }).click();
  await expect(page.getByText(`Übergang „${backLabel}“ angelegt.`)).toBeVisible();
  await expect(table.getByRole("row", { name: new RegExp(backLabel) })).toBeVisible();

  // "Anordnen" arranges anew: unsaved again.
  await page.getByRole("button", { name: "Anordnen" }).click();
  await expect(page.getByText("Anordnung verändert, noch nicht gespeichert.")).toBeVisible();

  // Over the API: a new record with priority 2 takes the transition right away.
  const created = await page.request.post("/api/v1/records/probe.probe", {
    headers,
    data: { title: `UI26 ${stamp}`, priority: 2 },
  });
  expect(created.status()).toBe(201);
  const id = ((await created.json()) as { id: string }).id;
  const moved = await page.request.post(`/api/v1/records/probe.probe/${id}:transition`, {
    headers,
    data: { transition: trName },
  });
  expect(moved.status()).toBe(200);
  expect(((await moved.json()) as { state: string }).state).toBe(stateName);

  // The export holds exactly the changes of this test: two by keyboard, the layout, the
  // dragged transition.
  const exported = (await (await page.request.get("/api/v1/customer")).json()) as {
    type: string;
    changes: { op: string; field: string }[];
  }[];
  const probe = exported.find((l) => l.type === "probe.probe");
  expect(probe?.changes.filter(mine("k_ui26_")).map((c) => c.op)).toEqual([
    "add_state",
    "add_transition",
    "set_layout",
    "add_transition",
  ]);

  // Back out: the record leaves the state over the dragged transition, then the changes go.
  const left = await page.request.post(`/api/v1/records/probe.probe/${id}:transition`, {
    headers,
    data: { transition: backName },
  });
  expect(left.status()).toBe(200);
  await takeBack(page, "k_ui26_");
});
