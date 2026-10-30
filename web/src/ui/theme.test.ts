// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

// Vitest runs with web/ as the working directory.
const src = (rel: string) => readFileSync(join(process.cwd(), "src", rel), "utf-8");

import { applyTheme, currentTheme, isTheme, themes } from "@/ui/theme";

describe("themes", () => {
  it("apply through the data attribute and survive a reload", () => {
    applyTheme("probe");
    expect(document.documentElement.dataset.theme).toBe("probe");
    expect(currentTheme()).toBe("probe");
    applyTheme("neutral");
    expect(document.documentElement.dataset.theme).toBeUndefined();
    expect(currentTheme()).toBe("neutral");
    expect(isTheme("nope")).toBe(false);
  });

  // Every theme but the default has a block in tokens.css, and every component colour is a
  // token: no hard-coded colour classes (bg-blue-500 and the like) in the components.
  it("are defined in tokens.css and the components use only tokens", () => {
    const css = src("ui/tokens.css");
    for (const theme of themes.filter((t) => t !== "neutral")) {
      expect(css).toContain(`[data-theme="${theme}"]`);
    }
    const files = ["button.tsx", "input.tsx", "label.tsx", "card.tsx", "alert.tsx"];
    for (const file of files) {
      const code = src(`ui/${file}`);
      expect(code, file).not.toMatch(/\b(bg|text|border)-(red|blue|green|gray|slate|zinc)-\d/);
    }
  });
});
