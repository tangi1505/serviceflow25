// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The accent colour (UI38, UI plan E41, E49, §2.3): six fixed colours without red, yellow and
// green, which mean deadline and state everywhere. A person's choice (or the default of the
// organisation, ui.accent) sets the accent of both modes: dark is the light colour mixed 58 %
// with 42 % white, the soft accent 10 % on white or 20 % on the dark card (§2.1); graphite gets
// the fixed dark value of --primary, because the mix only reaches 3.9 : 1. The tokens read the
// variables --pick-accent… (tokens.css); kept in the browser too, so that the next page starts
// in it before the settings are loaded.
export const accents = ["petrol", "steel", "indigo", "violet", "umber", "graphite"] as const;
export type Accent = (typeof accents)[number];

export const accentColors: Record<Accent, string> = {
  petrol: "#0B6E80",
  steel: "#1F5FA8",
  indigo: "#3F4BB8",
  violet: "#6B3FA0",
  umber: "#7A4E1D",
  graphite: "#15171A",
};

const storageKey = "itsm.accent";
const darkCard = "#181A1D";

export function isAccent(value: unknown): value is Accent {
  return typeof value === "string" && (accents as readonly string[]).includes(value);
}

function rgb(hex: string): [number, number, number] {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

// mix is share of a with the rest of b, as #rrggbb.
export function mix(a: string, b: string, share: number): string {
  const x = rgb(a);
  const y = rgb(b);
  return (
    "#" +
    x
      .map((v, i) => Math.round(v * share + y[i]! * (1 - share)))
      .map((v) => v.toString(16).padStart(2, "0"))
      .join("")
  );
}

// accentTokens are the four values of an accent: light, dark, and the soft one of each.
export function accentTokens(accent: Accent) {
  const light = accentColors[accent];
  return {
    light,
    dark: accent === "graphite" ? "#a9adb2" : mix(light, "#ffffff", 0.58),
    softLight: mix(light, "#ffffff", 0.1),
    softDark: mix(light, darkCard, 0.2),
  };
}

export function currentAccent(): Accent | null {
  try {
    const stored = localStorage.getItem(storageKey);
    return isAccent(stored) ? stored : null;
  } catch {
    return null;
  }
}

// applyAccent sets the accent on the page, before a theme's; null leaves the colours of the
// tokens.
export function applyAccent(accent: Accent | null): void {
  const style = document.documentElement.style;
  if (accent === null) {
    for (const name of ["light", "dark", "soft-light", "soft-dark"]) {
      style.removeProperty(`--pick-accent-${name}`);
    }
  } else {
    const t = accentTokens(accent);
    style.setProperty("--pick-accent-light", t.light);
    style.setProperty("--pick-accent-dark", t.dark);
    style.setProperty("--pick-accent-soft-light", t.softLight);
    style.setProperty("--pick-accent-soft-dark", t.softDark);
  }
  try {
    if (accent === null) {
      localStorage.removeItem(storageKey);
    } else {
      localStorage.setItem(storageKey, accent);
    }
  } catch {
    // Storage may be blocked; the accent still applies for this page.
  }
}
