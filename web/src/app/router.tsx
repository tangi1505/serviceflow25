// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Routes of the interface (TanStack Router, Hauptplan E41), code-based so that the tree is
// visible in one place. Everything under the shell needs a session; without one the user lands
// on /login, with an expired password on /password (E77).
import { QueryClient } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  redirect,
  type RouterHistory,
} from "@tanstack/react-router";

import { ApiError, meQuery } from "@/api/client";
import { AppShell } from "@/app/AppShell";
import { HomePage } from "@/app/HomePage";
import { LoginPage } from "@/app/LoginPage";
import { PasswordPage } from "@/app/PasswordPage";
import { GlobalSettingsPage, MySettingsPage } from "@/app/SettingsPage";
import {
  PortalFormPage,
  PortalHome,
  PortalNewPage,
  PortalRecordPage,
  PortalShell,
} from "@/portal/Portal";
import { NewRecordPage, RecordPage } from "@/render/RecordPage";
import { CustomizeOverview, CustomizePage } from "@/render/CustomizePage";
import { RolePage, RolesOverview } from "@/render/RolePage";
import { SearchPage } from "@/render/SearchPage";
import { WorkflowPage } from "@/render/WorkflowPage";
import { TypePage } from "@/render/TypePage";

type RouterContext = { queryClient: QueryClient };

const rootRoute = createRootRouteWithContext<RouterContext>()({ component: Outlet });

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  // ?error=… is where a sign-in through the provider lands when it failed (PL11).
  validateSearch: (search: Record<string, unknown>): { error?: string } =>
    typeof search.error === "string" ? { error: search.error } : {},
  component: LoginPage,
});

const shellRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "shell",
  beforeLoad: async ({ context, location }) => {
    let me;
    try {
      me = await context.queryClient.ensureQueryData(meQuery);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        throw redirect({ to: "/login" });
      }
      throw err;
    }
    if (me.password_expired && location.pathname !== "/password") {
      throw redirect({ to: "/password" });
    }
    // The workplace is for agents (UI10); everyone else goes to the portal, also by address.
    if (me.workplace === false && location.pathname !== "/password") {
      throw redirect({ to: "/portal" });
    }
  },
  component: AppShell,
});

// The portal for users (UI10): its own frame below /portal.
const portalRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/portal",
  beforeLoad: async ({ context }) => {
    let me;
    try {
      me = await context.queryClient.ensureQueryData(meQuery);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        throw redirect({ to: "/login" });
      }
      throw err;
    }
    if (me.password_expired) {
      throw redirect({ to: "/password" });
    }
  },
  component: PortalShell,
});

const portalIndexRoute = createRoute({
  getParentRoute: () => portalRoute,
  path: "/",
  component: PortalHome,
});

const portalSettingsRoute = createRoute({
  getParentRoute: () => portalRoute,
  path: "settings",
  component: MySettingsPage,
});

const portalFormRoute = createRoute({
  getParentRoute: () => portalRoute,
  path: "forms/$formId",
  component: PortalFormPage,
});

const portalNewRoute = createRoute({
  getParentRoute: () => portalRoute,
  path: "$typeName/new",
  component: PortalNewPage,
});

const portalRecordRoute = createRoute({
  getParentRoute: () => portalRoute,
  path: "$typeName/$id",
  component: PortalRecordPage,
});

const indexRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/",
  component: HomePage,
});

// ?search=<id> opens a saved search (UI4), so that a link shows the same view.
const typeRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/types/$typeName",
  validateSearch: (search: Record<string, unknown>): { search?: string } =>
    typeof search["search"] === "string" ? { search: search["search"] } : {},
  component: TypePage,
});

// The form of a record: /types/<type>/new creates, /types/<type>/<id> edits (UI2).
const newRecordRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/types/$typeName/new",
  component: NewRecordPage,
});

const recordRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/types/$typeName/$id",
  component: RecordPage,
});

// The type and field editor and its overview (UI25); the static segment wins over $id.
const customizeRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/types/$typeName/customize",
  component: CustomizePage,
});

// The workflow editor of a type (UI26.1).
const workflowRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/types/$typeName/workflow",
  component: WorkflowPage,
});

// The results of the search field in the head (UI30).
const searchRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/search",
  validateSearch: (search: Record<string, unknown>): { q?: string } =>
    typeof search["q"] === "string" ? { q: search["q"] } : {},
  component: SearchPage,
});

// The role editor (UI27): the roles and one role with its permissions and assignments.
const rolesRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/roles",
  component: RolesOverview,
});

const roleRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/roles/$name",
  component: RolePage,
});

const customizeOverviewRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/customize",
  component: CustomizeOverview,
});

const passwordRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/password",
  component: PasswordPage,
});

// One's own settings and the global ones (UI9).
const settingsRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/settings",
  component: MySettingsPage,
});

const globalSettingsRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: "/settings/global",
  component: GlobalSettingsPage,
});

const routeTree = rootRoute.addChildren([
  portalRoute.addChildren([
    portalIndexRoute,
    portalSettingsRoute,
    portalFormRoute,
    portalNewRoute,
    portalRecordRoute,
  ]),
  loginRoute,
  shellRoute.addChildren([
    indexRoute,
    typeRoute,
    newRecordRoute,
    customizeRoute,
    workflowRoute,
    recordRoute,
    customizeOverviewRoute,
    rolesRoute,
    roleRoute,
    searchRoute,
    passwordRoute,
    settingsRoute,
    globalSettingsRoute,
  ]),
]);

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
  });
}

// createAppRouter builds the router; tests pass a memory history and their own client.
export function createAppRouter(options?: { queryClient?: QueryClient; history?: RouterHistory }) {
  const queryClient = options?.queryClient ?? createQueryClient();
  return createRouter({
    routeTree,
    context: { queryClient },
    history: options?.history,
    defaultPreload: "intent",
    scrollRestoration: true,
  });
}

export function memoryHistory(path: string): RouterHistory {
  return createMemoryHistory({ initialEntries: [path] });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}
