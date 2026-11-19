// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// IBM Plex, shipped with the tool (UI29; OFL-1.1 allowed for fonts only, test/licenses).
import "@fontsource/ibm-plex-sans/400.css";
import "@fontsource/ibm-plex-sans/500.css";
import "@fontsource/ibm-plex-sans/600.css";
import "@fontsource/ibm-plex-mono/400.css";
import "@fontsource/ibm-plex-mono/500.css";
import "@/ui/tokens.css";

import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { initI18n } from "@/i18n";
import { applyAccent, applyPrimary, currentAccent, currentPrimary } from "@/ui/accent";
import { applyColorScheme, currentColorScheme } from "@/ui/colorScheme";
import { applyTheme, currentTheme } from "@/ui/theme";
import { createAppRouter, createQueryClient } from "@/app/router";
import { Toaster } from "@/ui/toast";

initI18n(navigator.language);
applyTheme(currentTheme());
// The last known scheme before the settings load, so that a dark page does not flash light.
applyColorScheme(currentColorScheme());
applyAccent(currentAccent());
applyPrimary(currentPrimary());

const queryClient = createQueryClient();
const router = createAppRouter({ queryClient });

const root = document.getElementById("root");
if (!root) {
  throw new Error("#root missing in index.html");
}
createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      {/* The messages at the bottom right, for the app and the portal (UI59). */}
      <Toaster />
    </QueryClientProvider>
  </StrictMode>,
);
