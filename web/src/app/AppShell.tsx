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
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { CommandPalette } from "@/app/CommandPalette";
import { LiveProvider } from "@/app/live";
import { Notifications } from "@/app/Notifications";
import { forget } from "@/app/recent";
import { LanguageSwitch, TenantSwitch, ThemeSwitch } from "@/app/Switches";
import { Button } from "@/ui/button";
import {
  api,
  labelOf,
  meQuery,
  mySettingsQuery,
  qualifiedName,
  savedSearchesQuery,
  typesQuery,
} from "@/api/client";
import { setDisplayTimeZone } from "@/render/fields";
import { applyColorScheme, isColorScheme } from "@/ui/colorScheme";
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
    // Light, dark or like the system (UI29).
    if (isColorScheme(s.color_scheme?.value)) {
      applyColorScheme(s.color_scheme.value);
    }
    setDisplayTimeZone(s.time_zone.value);
  }, [settings.data, i18n]);

  // The command line (UI31): Strg K from anywhere, or the search field of the head.
  const [command, setCommand] = useState(false);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCommand(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  // The saved searches one may read, own and shared, in the navigation (UI30).
  const saved = useQuery({ ...savedSearchesQuery, enabled: me.data !== undefined });
  // The types grouped by their package: the areas of the navigation.
  const areas = new Map<string, NonNullable<typeof types.data>>();
  for (const ty of types.data ?? []) {
    const pkg = ty.package ?? qualifiedName(ty).split(".")[0] ?? "";
    areas.set(pkg, [...(areas.get(pkg) ?? []), ty]);
  }

  const logout = async () => {
    if (me.data) {
      forget(me.data.id);
    }
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
          <Link to="/" className="text-lg font-semibold text-foreground">
            {t("app.title")}
          </Link>
          <button
            type="button"
            aria-haspopup="dialog"
            aria-keyshortcuts="Control+K"
            onClick={() => setCommand(true)}
            className="flex h-9 min-w-48 max-w-lg flex-1 items-center justify-between gap-2 rounded-sm border border-input bg-card px-3 text-left text-sm text-foreground-2"
          >
            <span>{t("command.open")}</span>
            <kbd className="rounded border px-1.5 font-mono text-xs">{t("command.shortcut")}</kbd>
          </button>
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
        {/* Below 768 px the navigation stands above the content (UI30). */}
        <div className="flex flex-1 flex-col md:flex-row">
          <nav
            aria-label={t("nav.types")}
            className="w-full border-b bg-surface-subtle p-4 md:w-56 md:flex-none md:border-r md:border-b-0"
          >
            {[...areas.entries()].map(([pkg, list]) => (
              <div key={pkg}>
                <h2 className="mb-2 mt-4 text-xs font-semibold uppercase text-foreground-2 first:mt-0">
                  {t(`area.${pkg}`, { defaultValue: pkg })}
                </h2>
                <ul className="flex flex-col gap-1">
                  {list.map((type) => {
                    const name = qualifiedName(type);
                    return (
                      <li key={name}>
                        <Link
                          to="/types/$typeName"
                          params={{ typeName: name }}
                          className="block rounded-md px-2 py-1 text-sm hover:bg-surface-active"
                          activeProps={{
                            className: "bg-surface-active font-medium text-foreground",
                          }}
                        >
                          {labelOf(type.label, i18n.language, name)}
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </div>
            ))}
            {types.isError && <p className="text-sm text-destructive">{t("nav.typesError")}</p>}
            {(saved.data?.items.length ?? 0) > 0 && (
              <>
                <h2 className="mb-2 mt-6 text-xs font-semibold uppercase text-foreground-2">
                  {t("nav.saved")}
                </h2>
                <ul className="flex flex-col gap-1">
                  {saved.data!.items.map((s) => (
                    <li key={String(s["id"])}>
                      <Link
                        to="/types/$typeName"
                        params={{ typeName: String(s["target_type"]) }}
                        search={{ search: String(s["id"]) }}
                        className="block rounded-md px-2 py-1 text-sm hover:bg-surface-active"
                      >
                        {String(s["name"])}
                      </Link>
                    </li>
                  ))}
                </ul>
              </>
            )}
            {(me.data?.customize === true || me.data?.manage_rights === true) && (
              <>
                <h2 className="mb-2 mt-6 text-xs font-semibold uppercase text-foreground-2">
                  {t("nav.admin")}
                </h2>
                <ul className="flex flex-col gap-1">
                  {me.data.manage_rights === true && (
                    <li>
                      <Link
                        to="/roles"
                        className="block rounded-md px-2 py-1 text-sm hover:bg-surface-active"
                        activeProps={{
                          className: "bg-surface-active font-medium text-foreground",
                        }}
                      >
                        {t("nav.roles")}
                      </Link>
                    </li>
                  )}
                  {me.data.customize === true && (
                    <li>
                      <Link
                        to="/customize"
                        className="block rounded-md px-2 py-1 text-sm hover:bg-surface-active"
                        activeProps={{
                          className: "bg-surface-active font-medium text-foreground",
                        }}
                      >
                        {t("nav.customize")}
                      </Link>
                    </li>
                  )}
                </ul>
              </>
            )}
          </nav>
          <main className="min-w-0 flex-1 p-4 md:p-6">
            <Outlet />
          </main>
        </div>
        {command && <CommandPalette onClose={() => setCommand(false)} />}
      </div>
    </LiveProvider>
  );
}
