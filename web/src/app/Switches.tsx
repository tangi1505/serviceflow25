// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Language and theme as native selects for the sign-in page and the portal: keyboard and screen
// reader support come for free. Signed in (persist), a choice is also kept in the person's
// settings (UI9), so that it holds on every device and after signing in again. The workplace
// sets mode and language in the account menu of its header (UI35).
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import { api, mySettingsQuery, type SettingsChange } from "@/api/client";
import { languages, type Language } from "@/i18n";
import { applyTheme, currentTheme, themeEvent, themes, isTheme, type Theme } from "@/ui/theme";
import { Label } from "@/ui/label";

const selectClass =
  "h-9 rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

// usePersist keeps a change in the settings of the person when persist is set.
export function usePersist(persist: boolean) {
  const queryClient = useQueryClient();
  return (change: SettingsChange) => {
    if (!persist) {
      return;
    }
    void api
      .setMySettings(change)
      .then((saved) => queryClient.setQueryData(mySettingsQuery.queryKey, saved))
      .catch(() => undefined);
  };
}

export function LanguageSwitch({ persist = false }: { persist?: boolean }) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const keep = usePersist(persist);
  const change = (lng: Language) => {
    void i18n.changeLanguage(lng);
    document.documentElement.lang = lng;
    keep({ language: lng });
  };
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id}>{t("nav.language")}</Label>
      <select
        id={id}
        className={selectClass}
        value={i18n.language}
        onChange={(e) => change(e.target.value as Language)}
      >
        {Object.keys(languages).map((lng) => (
          <option key={lng} value={lng}>
            {t(`language.${lng}`)}
          </option>
        ))}
      </select>
    </div>
  );
}

export function ThemeSwitch({ persist = false }: { persist?: boolean }) {
  const { t } = useTranslation();
  const id = useId();
  const keep = usePersist(persist);
  const [theme, setTheme] = useState<Theme>(currentTheme);
  useEffect(() => {
    const follow = (e: Event) => {
      const value = (e as CustomEvent<string>).detail;
      if (isTheme(value)) {
        setTheme(value);
      }
    };
    window.addEventListener(themeEvent, follow);
    return () => window.removeEventListener(themeEvent, follow);
  }, []);
  const change = (value: string) => {
    if (isTheme(value)) {
      applyTheme(value);
      setTheme(value);
      keep({ theme: value });
    }
  };
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id}>{t("nav.theme")}</Label>
      <select
        id={id}
        className={selectClass}
        value={theme}
        onChange={(e) => change(e.target.value)}
      >
        {themes.map((name) => (
          <option key={name} value={name}>
            {t(`theme.${name}`)}
          </option>
        ))}
      </select>
    </div>
  );
}
