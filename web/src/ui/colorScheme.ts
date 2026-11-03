// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The colour scheme (UI29, UI plan E24): light, dark or like the system of the device. The
// setting comes from the person (UI9 settings, color_scheme) and is kept in the browser too, so
// that the next page starts in it before the settings are loaded. "system" follows
// prefers-color-scheme, also when it changes while the page is open. The tokens read
// data-color-scheme on <html>.
export const colorSchemes = ["system", "light", "dark"] as const;
export type ColorScheme = (typeof colorSchemes)[number];

const storageKey = "itsm.color_scheme";
let stopFollowing: (() => void) | null = null;

export function isColorScheme(value: unknown): value is ColorScheme {
  return typeof value === "string" && (colorSchemes as readonly string[]).includes(value);
}

export function currentColorScheme(): ColorScheme {
  try {
    const stored = localStorage.getItem(storageKey);
    return isColorScheme(stored) ? stored : "system";
  } catch {
    return "system";
  }
}

function systemDark(): MediaQueryList | null {
  return typeof window.matchMedia === "function"
    ? window.matchMedia("(prefers-color-scheme: dark)")
    : null;
}

function set(dark: boolean) {
  document.documentElement.dataset.colorScheme = dark ? "dark" : "light";
}

export function applyColorScheme(scheme: ColorScheme): void {
  stopFollowing?.();
  stopFollowing = null;
  if (scheme === "system") {
    const media = systemDark();
    set(media?.matches === true);
    if (media) {
      const follow = (e: MediaQueryListEvent) => set(e.matches);
      media.addEventListener("change", follow);
      stopFollowing = () => media.removeEventListener("change", follow);
    }
  } else {
    set(scheme === "dark");
  }
  try {
    localStorage.setItem(storageKey, scheme);
  } catch {
    // Storage may be blocked; the scheme still applies for this page.
  }
}
