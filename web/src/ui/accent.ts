// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The accent colour (UI38, UI plan E41, E49, §2.3): six fixed colours without red, yellow and
// green, which mean deadline and state everywhere. A person's choice (or the colour of the
// organisation, ui.accent, since UI39 any colour with 4.5 : 1 on white, E51) sets the accent of
// both modes: dark is the light colour mixed 58 % with 42 % white, the soft accent 10 % on white
// or 20 % on the dark card (§2.1); where the dark one stays below 4.5 : 1 on the dark card, more
// white goes in until it reaches it (E51); graphite gets the fixed dark value of --primary. The
// tokens read the variables --pick-accent… (tokens.css); kept in the browser too, so that the
// next page starts in it before the settings are loaded. The main action may take the accent
// instead of graphite (E25, ui.primary_accent): --pick-primary….
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
const primaryKey = "itsm.primary";
const darkCard = "#181A1D";
const darkGround = "#111315";
const hexRe = /^#[0-9a-fA-F]{6}$/;

export function isAccent(value: unknown): value is Accent {
  return typeof value === "string" && (accents as readonly string[]).includes(value);
}

// isAccentValue says whether value names one of the six or is a colour #rrggbb.
export function isAccentValue(value: unknown): value is string {
  return isAccent(value) || (typeof value === "string" && hexRe.test(value));
}

// accentHex is the colour of a name or of #rrggbb.
export function accentHex(value: string): string {
  return isAccent(value) ? accentColors[value] : value;
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

// contrast is the contrast ratio of two colours after WCAG 2.1, from 1 to 21.
export function contrast(a: string, b: string): number {
  const lum = (hex: string) => {
    const [r, g, b2] = rgb(hex).map((v) => {
      const c = v / 255;
      return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    }) as [number, number, number];
    return 0.2126 * r + 0.7152 * g + 0.0722 * b2;
  };
  const x = lum(a);
  const y = lum(b);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

// accentTokens are the four values of an accent: light, dark, and the soft one of each.
export function accentTokens(value: string) {
  const light = accentHex(value);
  let dark = "#a9adb2";
  if (light.toLowerCase() !== accentColors.graphite.toLowerCase()) {
    let share = 0.58;
    dark = mix(light, "#ffffff", share);
    while (contrast(dark, darkCard) < 4.5 && share > 0) {
      share = Math.max(0, share - 0.02);
      dark = mix(light, "#ffffff", share);
    }
  }
  return {
    light,
    dark,
    softLight: mix(light, "#ffffff", 0.1),
    softDark: mix(light, darkCard, 0.2),
  };
}

export function currentAccent(): string | null {
  try {
    const stored = localStorage.getItem(storageKey);
    return isAccentValue(stored) ? stored : null;
  } catch {
    return null;
  }
}

function keep(key: string, value: string | null) {
  try {
    if (value === null) {
      localStorage.removeItem(key);
    } else {
      localStorage.setItem(key, value);
    }
  } catch {
    // Storage may be blocked; the colour still applies for this page.
  }
}

// applyAccent sets the accent on the page, before a theme's; null leaves the colours of the
// tokens.
export function applyAccent(value: string | null): void {
  const style = document.documentElement.style;
  const accent = isAccentValue(value) ? value : null;
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
  keep(storageKey, accent);
}

export function currentPrimary(): string | null {
  try {
    const stored = localStorage.getItem(primaryKey);
    return isAccentValue(stored) ? stored : null;
  } catch {
    return null;
  }
}

// applyPrimary puts the main action in the colour (E25, UI39); null leaves it graphite.
export function applyPrimary(value: string | null): void {
  const style = document.documentElement.style;
  const color = isAccentValue(value) ? value : null;
  const names = ["light", "dark", "fg-dark", "kbd-light", "kbd-dark"];
  if (color === null) {
    for (const name of names) {
      style.removeProperty(`--pick-primary-${name}`);
    }
  } else {
    const t = accentTokens(color);
    style.setProperty("--pick-primary-light", t.light);
    style.setProperty("--pick-primary-dark", t.dark);
    style.setProperty("--pick-primary-fg-dark", darkGround);
    style.setProperty("--pick-primary-kbd-light", mix(t.light, "#000000", 0.75));
    style.setProperty("--pick-primary-kbd-dark", mix(t.dark, darkGround, 0.75));
  }
  keep(primaryKey, color);
}
