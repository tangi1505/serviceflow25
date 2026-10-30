// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// German and English from the start (Hauptplan E42); no visible text in the code.
import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import de from "@/i18n/de.json";
import en from "@/i18n/en.json";

export const languages = { de, en } as const;
export type Language = keyof typeof languages;

export function pickLanguage(browserLanguage: string): Language {
  return browserLanguage.toLowerCase().startsWith("en") ? "en" : "de";
}

export function initI18n(browserLanguage: string): void {
  const lng = pickLanguage(browserLanguage);
  document.documentElement.lang = lng;
  void i18n.use(initReactI18next).init({
    resources: { de: { translation: de }, en: { translation: en } },
    lng,
    fallbackLng: "de",
    interpolation: { escapeValue: false },
  });
}
