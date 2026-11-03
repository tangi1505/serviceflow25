// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The marks of the kit (UI29) and the colour scheme: the text carries the meaning, the colour
// only adds; light, dark and "like the system", which follows the device while the page is open.
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { applyColorScheme, currentColorScheme } from "@/ui/colorScheme";
import { Deadline, PriorityMark, StatePill } from "@/ui/marks";
import { expectAccessible } from "@/test/setup";

describe("Marks", () => {
  it("say state, priority and time left in text", async () => {
    const view = render(
      <div>
        <StatePill label="In Arbeit" category="active" />
        <StatePill label="Neu" category="open" />
        <PriorityMark value={1} />
        <PriorityMark value={4} />
        <Deadline text="noch 2 h 10 min" share={0.8} state="close" />
      </div>,
    );
    expect(screen.getByText("In Arbeit")).toBeInTheDocument();
    expect(screen.getByText("P1")).toHaveClass("bg-primary");
    expect(screen.getByText("P4")).toHaveClass("text-muted-foreground");
    const time = screen.getByText("noch 2 h 10 min");
    expect(time).toHaveClass("text-warning-foreground");
    expect(time.nextElementSibling?.firstElementChild).toHaveStyle({ width: "80%" });
    await expectAccessible(view.container);
  });
});

describe("Colour scheme", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    applyColorScheme("light");
  });

  it("sets light and dark, keeps the choice and follows the system", () => {
    applyColorScheme("dark");
    expect(document.documentElement.dataset.colorScheme).toBe("dark");
    expect(currentColorScheme()).toBe("dark");

    let listener: ((e: { matches: boolean }) => void) | null = null;
    const media = {
      matches: false,
      addEventListener: (_: string, l: (e: { matches: boolean }) => void) => {
        listener = l;
      },
      removeEventListener: () => {
        listener = null;
      },
    };
    vi.stubGlobal("matchMedia", () => media);
    applyColorScheme("system");
    expect(document.documentElement.dataset.colorScheme).toBe("light");
    listener!({ matches: true });
    expect(document.documentElement.dataset.colorScheme).toBe("dark");
    // Choosing light again stops following the device.
    applyColorScheme("light");
    expect(listener).toBeNull();
    expect(document.documentElement.dataset.colorScheme).toBe("light");
  });
});
