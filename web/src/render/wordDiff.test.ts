// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The word diff of SD16b: common words stay, the old side strikes what went, the new side
// marks what came; empty and huge texts.
import { describe, expect, it } from "vitest";

import { wordDiff } from "./wordDiff";

describe("wordDiff", () => {
  it("marks the words that went and came", () => {
    const d = wordDiff("Starten Sie neu. Rufen Sie an.", "Starten Sie neu. Melden Sie sich ab.");
    expect(d.old.map((s) => [s.kind, s.text])).toEqual([
      ["same", "Starten Sie neu. "],
      ["del", "Rufen"],
      ["same", " Sie "],
      ["del", "an."],
    ]);
    expect(d.new.filter((s) => s.kind === "ins").map((s) => s.text)).toEqual([
      "Melden",
      "sich ab.",
    ]);
    expect(d.new.map((s) => s.text).join("")).toBe("Starten Sie neu. Melden Sie sich ab.");
  });

  it("handles empty and equal texts", () => {
    expect(wordDiff("", "neu")).toEqual({ old: [], new: [{ text: "neu", kind: "ins" }] });
    expect(wordDiff("gleich", "gleich").old).toEqual([{ text: "gleich", kind: "same" }]);
  });

  it("compares huge texts as a whole", () => {
    const big = Array.from({ length: 3000 }, (_, i) => `w${i}`).join(" ");
    const d = wordDiff(big, `${big} x`);
    expect(d.old).toEqual([{ text: big, kind: "del" }]);
    expect(d.new).toEqual([{ text: `${big} x`, kind: "ins" }]);
  });
});
