// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Themes are sets of design tokens (Hauptplan E62); the values live in styles/tokens.css under
// the data-theme attribute. "neutral" is the default and has no attribute.
export const themes = ["neutral", "probe"] as const;
export type Theme = (typeof themes)[number];

const storageKey = "itsm.theme";

export function isTheme(value: unknown): value is Theme {
  return typeof value === "string" && (themes as readonly string[]).includes(value);
}

export function currentTheme(): Theme {
  try {
    const stored = localStorage.getItem(storageKey);
    return isTheme(stored) ? stored : "neutral";
  } catch {
    return "neutral";
  }
}

export function applyTheme(theme: Theme): void {
  if (theme === "neutral") {
    delete document.documentElement.dataset.theme;
  } else {
    document.documentElement.dataset.theme = theme;
  }
  try {
    localStorage.setItem(storageKey, theme);
  } catch {
    // Storage may be blocked; the theme still applies for this page.
  }
  // The theme switch follows a theme set elsewhere (the settings of the person, UI9).
  window.dispatchEvent(new CustomEvent(themeEvent, { detail: theme }));
}

export const themeEvent = "itsm:theme";
