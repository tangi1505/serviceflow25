// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The select field (UI57): fully usable with the keyboard after the WAI-ARIA pattern
// "select-only combobox", a search field from ten options on, and the two heights.
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { expectAccessible } from "@/test/setup";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { Select, type SelectOption } from "@/ui/select";

const priorities: SelectOption[] = [
  { value: "1", label: "Niedrig" },
  { value: "2", label: "Mittel" },
  { value: "3", label: "Hoch" },
  { value: "4", label: "Kritisch", disabled: true },
];

function Field({ options = priorities, start = "" }: { options?: SelectOption[]; start?: string }) {
  const [value, setValue] = useState(start);
  return (
    <form>
      <button type="button">davor</button>
      <Label htmlFor="prio">Priorität</Label>
      <Select id="prio" value={value} onChange={setValue} options={options} placeholder="Wählen" />
      <output data-testid="value">{value}</output>
    </form>
  );
}

describe("Select", () => {
  it("is usable with the keyboard alone", async () => {
    const user = userEvent.setup();
    const { container } = render(<Field />);
    const box = screen.getByRole("combobox", { name: "Priorität" });
    expect(box).toHaveTextContent("Wählen");

    await user.click(screen.getByRole("button", { name: "davor" }));
    await user.keyboard("{Tab}");
    expect(box).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(box).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    await user.keyboard("h");
    expect(box.getAttribute("aria-activedescendant")).toBe(
      screen.getByRole("option", { name: "Hoch" }).id,
    );
    await expectAccessible(container);
    await user.keyboard("{Enter}");
    expect(screen.getByTestId("value")).toHaveTextContent("3");
    expect(box).toHaveTextContent("Hoch");
    expect(box).toHaveAttribute("aria-expanded", "false");
    expect(box).toHaveFocus();

    // Esc closes without a change and leaves the focus on the field.
    await user.keyboard("{ArrowDown}{ArrowUp}{Escape}");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.getByTestId("value")).toHaveTextContent("3");
    expect(box).toHaveFocus();

    // Home opens on the first option; a disabled option is skipped by the arrows.
    await user.keyboard("{Home}");
    expect(box.getAttribute("aria-activedescendant")).toBe(
      screen.getByRole("option", { name: "Niedrig" }).id,
    );
    await user.keyboard("{End}{Enter}");
    expect(screen.getByTestId("value")).toHaveTextContent("3");
    await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");
    expect(screen.getByTestId("value")).toHaveTextContent("3");
  });

  it("takes a click and closes on a click outside", async () => {
    const user = userEvent.setup();
    render(<Field start="1" />);
    const box = screen.getByRole("combobox", { name: "Priorität" });
    await user.click(box);
    expect(screen.getByRole("option", { name: "Niedrig" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await user.click(screen.getByRole("option", { name: "Mittel" }));
    expect(screen.getByTestId("value")).toHaveTextContent("2");
    await user.click(box);
    await user.click(screen.getByRole("button", { name: "davor" }));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("has a search field from ten options on", async () => {
    const user = userEvent.setup();
    const many = Array.from({ length: 12 }, (_, i) => ({
      value: String(i),
      label: `Gruppe ${String.fromCharCode(65 + i)}`,
    }));
    const { container } = render(<Field options={many} />);
    await user.click(screen.getByRole("combobox", { name: "Priorität" }));
    // Without the app the texts are their keys.
    const search = screen.getByRole("combobox", { name: "select.search" });
    expect(search).toHaveFocus();
    await user.keyboard("gruppe k");
    expect(screen.getAllByRole("option")).toHaveLength(1);
    await expectAccessible(container);
    await user.keyboard("{Enter}");
    expect(screen.getByTestId("value")).toHaveTextContent("10");
    expect(screen.getByRole("combobox", { name: "Priorität" })).toHaveFocus();
  });

  it("has the two heights of the tokens", () => {
    render(
      <>
        <Select
          label="Leiste"
          size="bar"
          value=""
          onChange={() => undefined}
          options={priorities}
        />
        <Input aria-label="Feld" />
        <Input aria-label="Suche" size="bar" />
      </>,
    );
    expect(screen.getByRole("combobox", { name: "Leiste" }).className).toContain(
      "h-[var(--control-bar)]",
    );
    expect(screen.getByRole("textbox", { name: "Feld" }).className).toContain(
      "h-[var(--control-form)]",
    );
    expect(screen.getByRole("textbox", { name: "Suche" }).className).toContain(
      "h-[var(--control-bar)]",
    );
  });
});
