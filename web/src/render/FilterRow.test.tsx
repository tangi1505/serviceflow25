// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The filter row (UI50, UI51) only with the keyboard: field, comparison and value with their
// suggestions; a comma for a second value; arrow left to the comparison; the backspace a step
// back; a click on a chip to change it; roles with "mir"; periods kept as time words.
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createAppRouter, createQueryClient, memoryHistory } from "@/app/router";
import { initI18n } from "@/i18n";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

import { parseDay, typedPeriods } from "./FilterRow";

function renderAt(path: string) {
  const queryClient = createQueryClient();
  const router = createAppRouter({ queryClient, history: memoryHistory(path) });
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe("FilterRow", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(9);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const lastWhere = () =>
    JSON.parse(
      state.calls.filter((c) => c.path === "/api/v1/records/probe.probe:query").at(-1)!.body!,
    ).where as unknown;

  it("reads typed periods and days", () => {
    expect(typedPeriods("letzte 5 t", false)).toEqual(["last:5:d"]);
    expect(typedPeriods("Letzte 5 Minuten", false)).toEqual(["last:5:m"]);
    // "m" can be minutes or months: both are offered.
    expect(typedPeriods("letzte 3 m", false)).toEqual(["last:3:m", "last:3:mo"]);
    expect(typedPeriods("nächste 2 tage", true)).toEqual(["next:2:d"]);
    expect(typedPeriods("nächste 2 tage", false)).toEqual([]);
    expect(typedPeriods("last 2 weeks", false)).toEqual(["last:2:w"]);
    expect(typedPeriods("letzte", false)).toEqual([]);
    expect(parseDay("1.11.2026")).toBe("2026-11-01");
    expect(parseDay("2026-11-01")).toBe("2026-11-01");
    expect(parseDay("11/01/2026")).toBeUndefined();
  });

  it("sets, widens, changes and takes back filters by keyboard", async () => {
    const user = userEvent.setup();
    const { container } = renderAt("/types/probe.probe");
    await screen.findByText("9 Datensätze");
    const row = screen.getByRole("combobox", { name: "Filter setzen" });
    await user.click(row);
    expect(row).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("listbox", { name: "Feld" })).toBeInTheDocument();
    await expectAccessible(container);

    // "stat", Enter, "arb", Enter: the comparison was chosen in advance.
    await user.keyboard("stat{Enter}");
    expect(screen.getByRole("listbox", { name: "Wert" })).toBeInTheDocument();
    await user.keyboard("arb{Enter}");
    expect(
      await screen.findByRole("button", { name: /Status\s*ist\s*In Arbeit/ }),
    ).toBeInTheDocument();
    await waitFor(() => expect(lastWhere()).toEqual({ field: "state", eq: "in_progress" }));

    // A comma takes a value and waits for another: one of them.
    await user.keyboard("art{Enter}frag,wun{Enter}");
    await waitFor(() =>
      expect(lastWhere()).toEqual({
        and: [
          { field: "state", eq: "in_progress" },
          { field: "kind", in: ["question", "wish"] },
        ],
      }),
    );
    expect(
      screen.getByRole("button", { name: /Art\s*ist\s*Frage\s*,\s*Wunsch/ }),
    ).toBeInTheDocument();

    // The backspace in the empty field takes the last filter back, value by value.
    await user.keyboard("{Backspace}");
    expect(screen.queryByRole("button", { name: /Art\s*ist/ })).toBeNull();
    await user.keyboard("{Backspace}{Enter}");
    await waitFor(() =>
      expect(lastWhere()).toEqual({
        and: [
          { field: "state", eq: "in_progress" },
          { field: "kind", eq: "question" },
        ],
      }),
    );

    // Arrow left goes to the comparison: "ist nicht".
    await user.keyboard("art{Enter}{ArrowLeft}nicht{Enter}stör{Enter}");
    await waitFor(() =>
      expect((lastWhere() as { and: unknown[] }).and).toContainEqual({
        field: "kind",
        neq: "fault",
      }),
    );

    // A click on a chip takes it back into the row; Enter keeps its value.
    await user.click(screen.getByRole("button", { name: /Status\s*ist\s*In Arbeit/ }));
    expect(row).toHaveFocus();
    await user.keyboard("{Backspace}erl{Enter}");
    expect(
      await screen.findByRole("button", { name: /Status\s*ist\s*Erledigt/ }),
    ).toBeInTheDocument();

    // The cross removes a filter.
    await user.click(screen.getByRole("button", { name: "Filter entfernen: Status" }));
    expect(screen.queryByRole("button", { name: /Status\s*ist/ })).toBeNull();

    // Esc closes the suggestions, a second Esc leaves the row and drops what was typed.
    await user.click(row);
    await user.keyboard("prio{Enter}");
    await user.keyboard("{Escape}");
    expect(row).toHaveAttribute("aria-expanded", "false");
    await user.keyboard("{Escape}");
    expect(row).not.toHaveFocus();
    await user.click(row);
    expect(screen.getByRole("listbox", { name: "Feld" })).toBeInTheDocument();
  });

  it("takes roles with mir and periods as time words", async () => {
    const user = userEvent.setup();
    renderAt("/types/probe.probe");
    await screen.findByText("9 Datensätze");
    await user.click(screen.getByRole("combobox", { name: "Filter setzen" }));

    // A role: "mir" (UI51).
    await user.keyboard("anford{Enter}mir{Enter}");
    expect(
      await screen.findByRole("button", { name: /Anforderer\s*ist\s*mir/ }),
    ).toBeInTheDocument();
    await waitFor(() => expect(lastWhere()).toEqual({ field: "actors.requester", eq: "@me" }));

    // "angelegt", Enter, "letzte 5 t", Enter: a period that runs with the clock.
    await user.keyboard("angel{Enter}letzte 5 t{Enter}");
    expect(
      await screen.findByRole("button", { name: /Angelegt:\s*letzte 5 Tage/ }),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect((lastWhere() as { and: unknown[] }).and).toContainEqual({
        field: "created_at",
        gte: "@now-5d",
      }),
    );

    // A suggested one: "gestern" is two comparisons.
    await user.keyboard("geänd{Enter}gest{Enter}");
    await waitFor(() =>
      expect((lastWhere() as { and: unknown[] }).and).toEqual(
        expect.arrayContaining([
          { field: "updated_at", gte: "@today-1d" },
          { field: "updated_at", lt: "@today" },
        ]),
      ),
    );
    expect(screen.getByRole("button", { name: /Geändert:\s*gestern/ })).toBeInTheDocument();

    // "ab" a day: arrow left, "ab", the day typed.
    await user.keyboard("gesehen{Enter}{ArrowLeft}ab{Enter}1.11.2026{Enter}");
    expect(
      await screen.findByRole("button", { name: /Gesehen\s*ab\s*1\.11\.2026/ }),
    ).toBeInTheDocument();
  });
});
