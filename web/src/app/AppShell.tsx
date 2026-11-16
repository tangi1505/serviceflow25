// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The frame after sign-in (UI1): header (UI35, Header.tsx), navigation with the areas (UI36,
// Nav.tsx), the page in the middle. The frame holds the one
// connection to the event stream of the tab (UI8).
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Outlet, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { CommandPalette } from "@/app/CommandPalette";
import { LiveProvider } from "@/app/live";
import { AppHeader } from "@/app/Header";
import { Nav } from "@/app/Nav";
import { forget } from "@/app/recent";
import { api, meQuery, mySettingsQuery } from "@/api/client";
import { useContentLanguage } from "@/app/contentLanguage";
import { setDisplayTimeZone } from "@/render/fields";
import { applyColorScheme, isColorScheme } from "@/ui/colorScheme";
import { applyTheme, isTheme } from "@/ui/theme";

export function AppShell() {
  const { i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  useContentLanguage();
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
      {/* From 768 px the header stays, and navigation and page scroll each for themselves (UI36);
          below, the navigation stands above the content (UI30) and the page scrolls. */}
      <div className="flex min-h-screen flex-col md:h-screen md:min-h-0">
        <AppHeader onCommand={() => setCommand(true)} onLogout={() => void logout()} />
        <div className="flex flex-1 flex-col md:min-h-0 md:flex-row">
          <Nav />
          <main className="min-w-0 flex-1 p-4 md:overflow-y-auto md:p-6">
            <Outlet />
          </main>
        </div>
        {command && <CommandPalette onClose={() => setCommand(false)} />}
      </div>
    </LiveProvider>
  );
}
