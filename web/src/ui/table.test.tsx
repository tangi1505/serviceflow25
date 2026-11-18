// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The plain table (UI54): the row height follows the density, a row that leads somewhere opens
// by click and Enter, the arrows, Pos1 and Ende move between rows, and a button inside a row
// does its own thing.
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { expectAccessible } from "@/test/setup";
import { Table, Td, Th, THead, Tr, type Density } from "@/ui/table";

function Sample({
  density,
  open,
  act,
}: {
  density: Density;
  open: (n: string) => void;
  act: () => void;
}) {
  return (
    <Table density={density} aria-label="Probe">
      <THead>
        <tr>
          <Th>Name</Th>
          <Th>
            <span className="sr-only">Aktion</span>
          </Th>
        </tr>
      </THead>
      <tbody>
        {["eins", "zwei", "drei"].map((n) => (
          <Tr key={n} onOpen={() => open(n)}>
            <Td>{n}</Td>
            <Td>
              <button type="button" onClick={act}>
                Entfernen {n}
              </button>
            </Td>
          </Tr>
        ))}
      </tbody>
    </Table>
  );
}

describe("Table", () => {
  it("opens rows by click and Enter and moves with the keys", async () => {
    const user = userEvent.setup();
    const open = vi.fn();
    const act = vi.fn();
    const { container } = render(<Sample density="normal" open={open} act={act} />);
    const rows = screen.getAllByRole("row").slice(1);
    expect(screen.getByText("eins")).toHaveStyle({ height: "52px" });

    await user.click(screen.getByText("zwei"));
    expect(open).toHaveBeenLastCalledWith("zwei");
    await user.click(screen.getByRole("button", { name: "Entfernen drei" }));
    expect(act).toHaveBeenCalledTimes(1);
    expect(open).toHaveBeenCalledTimes(1);

    rows[0]!.focus();
    await user.keyboard("{ArrowDown}");
    expect(rows[1]).toHaveFocus();
    await user.keyboard("{End}");
    expect(rows[2]).toHaveFocus();
    await user.keyboard("{Home}");
    expect(rows[0]).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(open).toHaveBeenLastCalledWith("eins");
    await expectAccessible(container);
  });

  it("is lower in the compact density", () => {
    render(<Sample density="compact" open={() => undefined} act={() => undefined} />);
    expect(screen.getByText("eins")).toHaveStyle({ height: "36px" });
  });
});
