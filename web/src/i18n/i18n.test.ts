// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

import { describe, expect, it } from "vitest";

import de from "@/i18n/de.json";
import en from "@/i18n/en.json";
import { pickLanguage } from "@/i18n/index";

function keys(obj: object, prefix = ""): string[] {
  return Object.entries(obj).flatMap(([k, v]) =>
    typeof v === "object" && v !== null ? keys(v, `${prefix}${k}.`) : [`${prefix}${k}`],
  );
}

describe("translations", () => {
  it("German and English have the same keys (E42)", () => {
    expect(keys(en).sort()).toEqual(keys(de).sort());
  });

  it("no translation is empty", () => {
    for (const lang of [de, en]) {
      for (const key of keys(lang)) {
        const value = key
          .split(".")
          .reduce<unknown>((o, k) => (o as Record<string, unknown>)[k], lang);
        expect(typeof value === "string" && value.trim() !== "", key).toBe(true);
      }
    }
  });
});

describe("pickLanguage", () => {
  it.each([
    ["en-US", "en"],
    ["EN", "en"],
    ["de-DE", "de"],
    ["fr-FR", "de"],
  ])("%s -> %s", (input, want) => {
    expect(pickLanguage(input)).toBe(want);
  });
});
