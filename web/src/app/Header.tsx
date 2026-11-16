// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The header of the workplace (UI35, UI plan E41, E47): brand, tenant as a button with a menu
// (only with more than one tenant), the search field of the command line in the middle, the
// bell with the unread of the inbox (WF19) and the account circle. Everything the person sets
// for herself hangs at the account circle; the theme is chosen in Meine Einstellungen only.
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  Bell,
  Building,
  Check,
  ChevronDown,
  ExternalLink,
  LayoutGrid,
  LogOut,
  Menu,
  Monitor,
  Moon,
  Search,
  Settings,
  Sun,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  inboxQuery,
  labelOf,
  meQuery,
  myBrandingQuery,
  mySettingsQuery,
  qualifiedName,
  typesQuery,
  type InboxEntry,
} from "@/api/client";
import { BrandMark, defaultName, useBranding } from "@/app/Brand";
import { useBrowserNotifications } from "@/app/Notifications";
import { usePersist } from "@/app/Switches";
import { languages, type Language } from "@/i18n";
import { formatMoment } from "@/render/fields";
import {
  applyColorScheme,
  colorSchemes,
  currentColorScheme,
  isColorScheme,
  type ColorScheme,
} from "@/ui/colorScheme";
import { menuKeys, usePopover } from "@/ui/popover";

const iconButton =
  "inline-grid size-11 flex-none place-items-center rounded-md text-foreground-2 hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
const panelClass = "absolute top-full z-50 mt-1 rounded-md border border-input bg-card shadow-lg";
const rowClass =
  "flex w-full items-center gap-2.5 rounded-sm px-2 text-left text-sm hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

// initials are the first letters of the first and the last word of a name.
export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) {
    return "?";
  }
  const first = words[0]![0] ?? "";
  const last = words.length > 1 ? (words[words.length - 1]![0] ?? "") : "";
  return (first + last).toUpperCase();
}

// The id of the button that opens the navigation on a phone; the drawer gives the focus back
// to it (UI40).
export const navOpenerId = "nav-opener";

export function AppHeader({
  onCommand,
  onLogout,
  onMenu,
  menuOpen = false,
}: {
  onCommand: () => void;
  onLogout: () => void;
  // onMenu opens the navigation on a phone (UI40, UI plan E42); without it the header is the
  // one of a wide window.
  onMenu?: () => void;
  menuOpen?: boolean;
}) {
  const { t } = useTranslation();
  const branding = useQuery(myBrandingQuery);
  useBranding(branding.data);
  const phone = onMenu !== undefined;
  return (
    <header
      className={`relative flex h-14 flex-none items-center border-b bg-card ${phone ? "gap-1 pr-2 pl-1" : "gap-3 pr-3 pl-4"}`}
    >
      <div className="flex min-w-0 flex-1 items-center gap-1.5">
        {phone && (
          <button
            id={navOpenerId}
            type="button"
            aria-label={t("nav.open")}
            aria-expanded={menuOpen}
            onClick={onMenu}
            className={iconButton}
          >
            <Menu aria-hidden="true" className="size-5" />
          </button>
        )}
        <Link to="/" className="flex flex-none items-center gap-2.5 py-1 pr-1.5 text-foreground">
          {/* Logo and name of the organisation (UI39). */}
          <BrandMark branding={branding.data} />
          <span className="text-[15px] font-semibold tracking-tight">
            {branding.data?.name ?? defaultName}
          </span>
        </Link>
        {/* On a phone the tenant stands at the top of the drawer (E42). */}
        {!phone && <TenantMenu />}
      </div>
      {phone ? (
        <button
          type="button"
          aria-haspopup="dialog"
          aria-label={t("command.open")}
          title={t("command.open")}
          onClick={onCommand}
          className={iconButton}
        >
          <Search aria-hidden="true" className="size-5" />
        </button>
      ) : (
        <button
          type="button"
          aria-haspopup="dialog"
          aria-keyshortcuts="Control+K"
          onClick={onCommand}
          className="flex h-9 min-w-0 flex-[0_1_520px] items-center gap-2.5 rounded-md border bg-background pr-2 pl-3 text-left text-sm text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Search aria-hidden="true" className="size-4 flex-none" />
          <span className="min-w-0 flex-1 truncate">{t("command.open")}</span>
          <kbd className="hidden rounded border border-b-2 border-input bg-card px-1.5 font-mono text-xs text-foreground-2 sm:inline">
            {t("command.shortcut")}
          </kbd>
        </button>
      )}
      <div className={`flex items-center justify-end gap-1 ${phone ? "flex-none" : "flex-1"}`}>
        <InboxBell />
        <AccountMenu onLogout={onLogout} />
      </div>
    </header>
  );
}

// TenantMenu lets an account of the installation change the tenant it works in (PL44); an
// account of one tenant does not see it. After the switch nothing of the old tenant stays in
// the cache, and the page starts at home. On a phone it stands in the drawer (UI40).
export function TenantMenu() {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const me = useQuery(meQuery);
  const pop = usePopover();
  const tenants = me.data?.tenants ?? [];
  if (tenants.length < 2) {
    return null;
  }
  const current = tenants.find((x) => x.path === me.data?.tenant);
  const name = labelOf(current?.label, i18n.language, me.data?.tenant ?? "");
  const change = async (tenant: string) => {
    pop.close();
    if (tenant === me.data?.tenant) {
      return;
    }
    const next = await api.setTenant(tenant);
    queryClient.setQueryData(meQuery.queryKey, next);
    queryClient.removeQueries({ predicate: (q) => q.queryKey[0] !== "me" });
    await navigate({ to: "/" });
  };
  return (
    <div className="relative flex min-w-0 items-center">
      <span aria-hidden="true" className="mx-1 h-6 w-px flex-none bg-border" />
      <button
        ref={pop.trigger}
        type="button"
        aria-haspopup="menu"
        aria-expanded={pop.open}
        aria-label={t("header.tenant", { name })}
        onClick={pop.toggle}
        className="inline-flex h-9 min-w-0 items-center gap-2 rounded-md pr-2 pl-2.5 text-sm font-medium text-foreground hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Building aria-hidden="true" className="size-4 flex-none text-foreground-2" />
        <span className="truncate">{name}</span>
        <ChevronDown aria-hidden="true" className="size-4 flex-none text-foreground-2" />
      </button>
      {pop.open && (
        <div
          ref={pop.panel}
          role="menu"
          tabIndex={-1}
          aria-label={t("header.tenants")}
          onKeyDown={menuKeys}
          className={`${panelClass} left-3 flex w-66 flex-col gap-px p-2`}
        >
          <p className="px-2 pt-1 pb-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {t("nav.tenant")}
          </p>
          {tenants.map((tenant) => {
            const on = tenant.path === me.data?.tenant;
            return (
              <button
                key={tenant.path}
                type="button"
                role="menuitemradio"
                aria-checked={on}
                onClick={() => void change(tenant.path)}
                className={`${rowClass} h-9 ${on ? "bg-accent-soft font-medium" : ""}`}
              >
                <span className="flex-1">{labelOf(tenant.label, i18n.language, tenant.path)}</span>
                {on && <Check aria-hidden="true" className="size-4 text-primary" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

// InboxBell shows the unread of the inbox (WF19, E47); its window lists the ten newest
// entries. Opening one marks it read.
function InboxBell() {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  const types = useQuery(typesQuery);
  const inbox = useQuery({ ...inboxQuery, enabled: me.data !== undefined });
  const browser = useBrowserNotifications();
  const pop = usePopover();
  const id = useId();
  const unread = inbox.data?.unread ?? 0;
  const read = async (body: { ids?: string[]; all?: boolean }) => {
    const left = await api.readInbox(body);
    queryClient.setQueryData(inboxQuery.queryKey, (old) =>
      old
        ? {
            ...old,
            unread: left.unread,
            items: old.items.map((e) =>
              body.all || body.ids?.includes(e.id)
                ? { ...e, read_at: e.read_at ?? new Date().toISOString() }
                : e,
            ),
          }
        : old,
    );
  };
  const typeLabel = (e: InboxEntry) =>
    labelOf(
      types.data?.find((x) => qualifiedName(x) === e.type_name)?.label,
      i18n.language,
      e.type_name,
    );
  return (
    <div className="relative">
      <button
        ref={pop.trigger}
        type="button"
        aria-haspopup="dialog"
        aria-expanded={pop.open}
        aria-label={unread > 0 ? t("header.bellUnread", { count: unread }) : t("header.bell")}
        onClick={pop.toggle}
        className={`${iconButton} relative`}
      >
        <Bell aria-hidden="true" className="size-5" />
        {unread > 0 && (
          <span
            aria-hidden="true"
            className="absolute top-1 right-0.5 h-4 min-w-4 rounded-full border-2 border-card bg-destructive px-1 font-mono text-[10px] leading-3 font-medium text-destructive-foreground"
          >
            {unread > 99 ? "99+" : unread}
          </span>
        )}
      </button>
      {pop.open && (
        <div
          ref={pop.panel}
          role="dialog"
          aria-labelledby={`${id}-title`}
          className={`${panelClass} right-0 flex w-96 max-w-[calc(100vw-16px)] flex-col`}
        >
          <div className="flex items-center justify-between gap-2 border-b px-3.5 py-2.5">
            <h2 id={`${id}-title`} className="text-sm font-semibold">
              {t("header.inbox")}
            </h2>
            <button
              type="button"
              disabled={unread === 0}
              onClick={() => void read({ all: true })}
              className="rounded-sm px-1.5 py-1 text-sm text-primary hover:underline disabled:text-muted-foreground disabled:no-underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {t("header.allRead")}
            </button>
          </div>
          {(inbox.data?.items.length ?? 0) === 0 ? (
            <p className="px-3.5 py-6 text-center text-sm text-muted-foreground">
              {inbox.isError ? t("header.inboxError") : t("header.inboxEmpty")}
            </p>
          ) : (
            <ul className="max-h-[60vh] overflow-y-auto p-1.5">
              {inbox.data!.items.map((e) => {
                const isNew = !e.read_at;
                return (
                  <li key={e.id}>
                    <Link
                      to="/types/$typeName/$id"
                      params={{ typeName: e.type_name, id: e.record_id }}
                      onClick={() => {
                        pop.close(false);
                        if (isNew) {
                          void read({ ids: [e.id] });
                        }
                      }}
                      className="flex gap-2.5 rounded-sm px-2 py-2 hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      <span
                        aria-hidden="true"
                        className={`mt-1.5 size-2 flex-none rounded-full ${isNew ? "bg-primary" : ""}`}
                      />
                      <span className="min-w-0 flex-1">
                        <span
                          className={`block truncate text-sm ${isNew ? "font-semibold text-foreground" : "text-foreground"}`}
                        >
                          {isNew && <span className="sr-only">{t("header.unread")} </span>}
                          {e.subject}
                        </span>
                        <span className="block truncate text-xs text-muted-foreground">
                          {[typeLabel(e), formatMoment(e.created_at, i18n.language), e.actor]
                            .filter(Boolean)
                            .join(" · ")}
                        </span>
                      </span>
                    </Link>
                  </li>
                );
              })}
            </ul>
          )}
          {browser.canAsk && (
            <div className="border-t p-1.5">
              <button
                type="button"
                onClick={browser.ask}
                className={`${rowClass} h-9 text-foreground-2`}
              >
                <Bell aria-hidden="true" className="size-4" />
                {t("live.allow")}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

const schemeIcons: Record<ColorScheme, LucideIcon> = { system: Monitor, light: Sun, dark: Moon };

// AccountMenu is the account circle and its window: mode and language at once, Meine
// Einstellungen, the portal and signing out.
export function AccountMenu({
  onLogout,
  portal = false,
}: {
  onLogout: () => void;
  // portal is the menu of the head of the portal (UI41, UI plan E43).
  portal?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const me = useQuery(meQuery);
  const settings = useQuery(mySettingsQuery);
  const keep = usePersist(true);
  const pop = usePopover();
  const id = useId();
  const [scheme, setScheme] = useState<ColorScheme>(currentColorScheme);
  const own = settings.data?.color_scheme?.value;
  useEffect(() => {
    if (isColorScheme(own)) {
      setScheme(own);
    }
  }, [own]);
  const name = me.data?.display_name ?? "";
  const tenant =
    (me.data?.tenants?.length ?? 0) > 1
      ? labelOf(me.data?.tenants?.find((x) => x.path === me.data?.tenant)?.label, i18n.language, "")
      : "";
  const pickScheme = (value: ColorScheme) => {
    applyColorScheme(value);
    setScheme(value);
    keep({ color_scheme: value });
  };
  const pickLanguage = (lng: Language) => {
    void i18n.changeLanguage(lng);
    document.documentElement.lang = lng;
    keep({ language: lng });
  };
  const segment = (on: boolean) =>
    `inline-grid h-7 place-items-center rounded-sm text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
      on ? "bg-segment-active font-medium text-foreground shadow-sm" : "text-foreground-2"
    }`;
  return (
    <div className="relative">
      <button
        ref={pop.trigger}
        type="button"
        aria-haspopup="dialog"
        aria-expanded={pop.open}
        aria-label={t("header.account", { name })}
        onClick={pop.toggle}
        className="inline-grid size-11 place-items-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <span
          aria-hidden="true"
          className="inline-grid size-8 place-items-center rounded-full bg-primary text-xs font-semibold text-primary-foreground"
        >
          {initials(name)}
        </span>
      </button>
      {pop.open && (
        <div
          ref={pop.panel}
          role="dialog"
          aria-label={t("header.accountTitle")}
          className={`${panelClass} right-0 flex w-80 max-w-[calc(100vw-16px)] flex-col`}
        >
          <div className="flex items-center gap-3 px-3.5 pt-3.5 pb-3">
            <span
              aria-hidden="true"
              className="inline-grid size-10 flex-none place-items-center rounded-full bg-primary font-semibold text-primary-foreground"
            >
              {initials(name)}
            </span>
            <div className="min-w-0">
              <p className="font-semibold">{name}</p>
              <p className="truncate text-xs text-muted-foreground">
                {[me.data?.username, tenant].filter(Boolean).join(" · ")}
              </p>
            </div>
          </div>
          <div className="flex flex-col gap-2.5 border-t px-3.5 py-3">
            {/* Only where the organisation lets the person choose the mode (UI39). */}
            {settings.data?.allowed?.color_scheme !== false && (
              <div className="flex items-center justify-between gap-3">
                <span id={`${id}-mode`} className="text-sm text-foreground-2">
                  {t("header.mode")}
                </span>
                <div
                  role="group"
                  aria-labelledby={`${id}-mode`}
                  className="inline-flex gap-0.5 rounded-md bg-muted p-0.5"
                >
                  {colorSchemes.map((value) => {
                    const Icon = schemeIcons[value];
                    return (
                      <button
                        key={value}
                        type="button"
                        aria-pressed={scheme === value}
                        aria-label={t(`colorScheme.${value}`)}
                        title={t(`colorScheme.${value}`)}
                        onClick={() => pickScheme(value)}
                        className={`${segment(scheme === value)} w-10`}
                      >
                        <Icon aria-hidden="true" className="size-4" />
                      </button>
                    );
                  })}
                </div>
              </div>
            )}
            <div className="flex items-center justify-between gap-3">
              <span id={`${id}-lang`} className="text-sm text-foreground-2">
                {t("nav.language")}
              </span>
              <div
                role="group"
                aria-labelledby={`${id}-lang`}
                className="inline-flex gap-0.5 rounded-md bg-muted p-0.5"
              >
                {(Object.keys(languages) as Language[]).map((lng) => (
                  <button
                    key={lng}
                    type="button"
                    lang={lng}
                    aria-pressed={i18n.language === lng}
                    onClick={() => pickLanguage(lng)}
                    className={`${segment(i18n.language === lng)} px-3`}
                  >
                    {t(`language.${lng}`)}
                  </button>
                ))}
              </div>
            </div>
          </div>
          <div className="flex flex-col gap-px border-t p-1.5">
            <Link
              to={portal ? "/portal/settings" : "/settings"}
              onClick={() => pop.close(false)}
              className={`${rowClass} h-11`}
            >
              <Settings aria-hidden="true" className="size-4 text-foreground-2" />
              {t("header.settings")}
            </Link>
            {portal ? (
              me.data?.workplace === true && (
                <Link to="/" onClick={() => pop.close(false)} className={`${rowClass} h-11`}>
                  <ExternalLink aria-hidden="true" className="size-4 text-foreground-2" />
                  {t("portal.toWorkplace")}
                </Link>
              )
            ) : (
              <Link to="/portal" onClick={() => pop.close(false)} className={`${rowClass} h-11`}>
                <LayoutGrid aria-hidden="true" className="size-4 text-foreground-2" />
                {t("portal.toPortal")}
              </Link>
            )}
          </div>
          <div className="border-t p-1.5">
            <button
              type="button"
              onClick={() => {
                pop.close(false);
                onLogout();
              }}
              className={`${rowClass} h-11`}
            >
              <LogOut aria-hidden="true" className="size-4 text-foreground-2" />
              {t("nav.logout")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
