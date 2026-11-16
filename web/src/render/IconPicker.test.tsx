// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The grid of symbols (UI42, UI plan E53): a group with its legend, "Ohne" and the symbols of
// the list by name, chosen with the mouse or the arrows.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { packageIcons } from "@/app/icons";
import { initI18n } from "@/i18n";
import { expectAccessible } from "@/test/setup";

import { IconPicker } from "./IconPicker";

function Picker({ start = "" }: { start?: string }) {
  const [value, setValue] = useState(start);
  return (
    <>
      <IconPicker id="tile-icon" legend="Symbol" value={value} onChange={setValue} />
      <output>{value || "–"}</output>
    </>
  );
}

describe("IconPicker", () => {
  beforeEach(() => {
    initI18n("de");
  });

  it("offers the symbols by name and takes a click or the arrows", async () => {
    const user = userEvent.setup();
    const { container } = render(<Picker />);
    const group = screen.getByRole("group", { name: "Symbol" });
    const radios = within(group).getAllByRole("radio");
    expect(radios).toHaveLength(Object.keys(packageIcons).length + 1);
    expect(within(group).getByRole("radio", { name: "Ohne" })).toBeChecked();
    await user.click(within(group).getByRole("radio", { name: "Telefon" }));
    expect(screen.getByRole("status")).toHaveTextContent("phone");
    expect(within(group).getByRole("radio", { name: "Telefon" })).toBeChecked();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("status")).toHaveTextContent("printer");
    await expectAccessible(container);
  });
});
