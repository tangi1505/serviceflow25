// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The frame after sign-in (UI1): header with the account, navigation with the installed types
// (the list of a type comes with UI2), the page in the middle. The frame holds the one
// connection to the event stream of the tab (UI8).
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";

import { LiveProvider } from "@/app/live";
import { Notifications } from "@/app/Notifications";
import { LanguageSwitch, TenantSwitch, ThemeSwitch } from "@/app/Switches";
import { Button } from "@/ui/button";
import { api, labelOf, meQuery, mySettingsQuery, qualifiedName, typesQuery } from "@/api/client";
import { setDisplayTimeZone } from "@/render/fields";
import { applyTheme, isTheme } from "@/ui/theme";

export function AppShell() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  const types = useQuery(typesQuery);
  // The person's settings (UI9): language, theme and time zone, once they are known.
  const settings = useQuery(mySettingsQuery);
  useEffect(() => {
    const s = settings.data;
    if (!s) {
      return;
    }
    if (s.language.value !== i18n.language) {
      void i18n.changeLanguage(s.language.value);
      document.documentElement.lang = s.language.value;
    }
    if (isTheme(s.theme.value)) {
      applyTheme(s.theme.value);
    }
    setDisplayTimeZone(s.time_zone.value);
  }, [settings.data, i18n]);

  const logout = async () => {
    try {
      await api.logout();
    } finally {
      queryClient.clear();
      await navigate({ to: "/login" });
    }
  };

  return (
    <LiveProvider>
      <div className="flex min-h-screen flex-col">
        <header className="flex flex-wrap items-center gap-4 border-b bg-card px-4 py-2">
          <Link to="/" className="text-lg font-semibold text-primary">
            {t("app.title")}
          </Link>
          <div className="ml-auto flex flex-wrap items-center gap-4">
            <Notifications />
            <Link to="/portal" className="text-sm underline">
              {t("portal.toPortal")}
            </Link>
            <TenantSwitch />
            <LanguageSwitch persist />
            <ThemeSwitch persist />
            {me.data && (
              <Link to="/settings" className="text-sm text-muted-foreground underline">
                {t("nav.user", { name: me.data.display_name })}
              </Link>
            )}
            <Button variant="outline" size="sm" onClick={() => void logout()}>
              {t("nav.logout")}
            </Button>
          </div>
        </header>
        <div className="flex flex-1">
          <nav aria-label={t("nav.types")} className="w-56 border-r bg-muted/40 p-4">
            <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">
              {t("nav.types")}
            </h2>
            <ul className="flex flex-col gap-1">
              {(types.data ?? []).map((type) => {
                const name = qualifiedName(type);
                return (
                  <li key={name}>
                    <Link
                      to="/types/$typeName"
                      params={{ typeName: name }}
                      className="block rounded-md px-2 py-1 text-sm hover:bg-muted"
                      activeProps={{
                        className: "bg-primary text-primary-foreground hover:bg-primary",
                      }}
                    >
                      {labelOf(type.label, i18n.language, name)}
                    </Link>
                  </li>
                );
              })}
            </ul>
            {types.isError && <p className="text-sm text-destructive">{t("nav.typesError")}</p>}
          </nav>
          <main className="flex-1 p-6">
            <Outlet />
          </main>
        </div>
      </div>
    </LiveProvider>
  );
}
