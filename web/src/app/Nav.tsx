// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The navigation of the workplace (UI36, UI plan E41, E48): Start and Zu genehmigen on top, then
// the areas with a symbol to open and close: Dashboards, one per package with its label and
// symbol from the package, the saved searches and the administration. Closed, an area shows
// how many entries it has. The bar scrolls for itself; its foot stays. Narrow (a setting of
// the person on the server), it is 60 px of symbols, and a click shows the entries beside it.
// Open and closed areas are kept in the browser per account. Below 768 px the navigation
// stands above the content (UI30) until UI40.
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useLocation } from "@tanstack/react-router";
import {
  Bookmark,
  ChevronDown,
  House,
  LayoutDashboard,
  ListChecks,
  PanelLeftClose,
  PanelLeftOpen,
  Shield,
  Star,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useId, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  dashboardsQuery,
  labelOf,
  meQuery,
  myApprovalsQuery,
  mySettingsQuery,
  packagesQuery,
  qualifiedName,
  savedSearchesQuery,
  typesQuery,
} from "@/api/client";
import { NewDashboardButton } from "@/app/Dashboard";
import { packageIcon } from "@/app/icons";
import { usePersist } from "@/app/Switches";
import { usePopover } from "@/ui/popover";

// An entry of an area: a link with its route; id is the area and the key, as kept among the
// favourites (UI37).
export type Entry = { id: string; key: string; label: string; link: ReactNode };
export type Area = {
  id: string;
  label: string;
  icon: LucideIcon;
  entries: Entry[];
  add?: ReactNode;
};
type RawArea = Omit<Area, "entries"> & { entries: Omit<Entry, "id">[] };

const itemClass =
  "block truncate rounded-md py-1.5 pr-1 pl-9 text-sm text-foreground hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
const activeClass = "bg-surface-active font-medium text-foreground";
const headClass =
  "flex h-9 items-center gap-2.5 rounded-md px-2 text-sm font-medium text-foreground hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

// openKey is where the open and closed areas of an account are kept in the browser.
function openKey(account: string) {
  return `itsm.nav.open.${account}`;
}

function readOpen(account: string): Record<string, boolean> {
  try {
    const raw = localStorage.getItem(openKey(account));
    return raw ? (JSON.parse(raw) as Record<string, boolean>) : {};
  } catch {
    return {};
  }
}

// useDesktop says whether the window is at least 768 px wide; only there the bar can be
// narrow. Without matchMedia (tests) it is.
const desktopQuery = "(min-width: 768px)";
function useDesktop(): boolean {
  const media = () =>
    typeof window.matchMedia === "function" ? window.matchMedia(desktopQuery) : null;
  const [desktop, setDesktop] = useState(() => media()?.matches ?? true);
  useEffect(() => {
    const m = media();
    if (!m) {
      return;
    }
    const follow = () => setDesktop(m.matches);
    m.addEventListener("change", follow);
    return () => m.removeEventListener("change", follow);
  }, []);
  return desktop;
}

// useAreas builds the areas from what the session may see, in the usual order.
export function useAreas(): Area[] {
  const { t, i18n } = useTranslation();
  const me = useQuery(meQuery);
  const signedIn = me.data !== undefined;
  const types = useQuery(typesQuery);
  const packages = useQuery({ ...packagesQuery, enabled: signedIn });
  const saved = useQuery({ ...savedSearchesQuery, enabled: signedIn });
  const dashboards = useQuery({ ...dashboardsQuery, enabled: signedIn });

  const areas: RawArea[] = [];
  areas.push({
    id: "dashboards",
    label: t("nav.dashboards"),
    icon: LayoutDashboard,
    add: <NewDashboardButton small />,
    entries: (dashboards.data?.items ?? []).map((b) => ({
      key: String(b["id"]),
      label: String(b["name"]),
      link: (
        <Link
          to="/dashboards/$id"
          params={{ id: String(b["id"]) }}
          className={itemClass}
          activeProps={{ className: activeClass }}
        >
          {String(b["name"])}
        </Link>
      ),
    })),
  });
  // One area per package, named and drawn by the package (E48): the others by their name,
  // the core (Stammdaten) after them.
  const byPackage = new Map<string, NonNullable<typeof types.data>>();
  for (const ty of types.data ?? []) {
    const pkg = ty.package ?? qualifiedName(ty).split(".")[0] ?? "";
    byPackage.set(pkg, [...(byPackage.get(pkg) ?? []), ty]);
  }
  const info = new Map((packages.data ?? []).map((p) => [p.name, p]));
  const packageAreas: RawArea[] = [...byPackage.entries()].map(([pkg, list]) => ({
    id: `package.${pkg}`,
    label: labelOf(info.get(pkg)?.label, i18n.language, pkg),
    icon: packageIcon(info.get(pkg)?.icon),
    entries: list.map((type) => {
      const name = qualifiedName(type);
      const label = labelOf(type.label, i18n.language, name);
      return {
        key: name,
        label,
        link: (
          <Link
            to="/types/$typeName"
            params={{ typeName: name }}
            className={itemClass}
            activeProps={{ className: activeClass }}
          >
            {label}
          </Link>
        ),
      };
    }),
  }));
  packageAreas.sort((a, b) =>
    a.id === "package.core"
      ? 1
      : b.id === "package.core"
        ? -1
        : a.label.localeCompare(b.label, i18n.language),
  );
  areas.push(...packageAreas);
  if ((saved.data?.items.length ?? 0) > 0) {
    areas.push({
      id: "saved",
      label: t("nav.saved"),
      icon: Bookmark,
      entries: saved.data!.items.map((s) => ({
        key: String(s["id"]),
        label: String(s["name"]),
        link: (
          <Link
            to="/types/$typeName"
            params={{ typeName: String(s["target_type"]) }}
            search={{ search: String(s["id"]) }}
            className={itemClass}
          >
            {String(s["name"])}
          </Link>
        ),
      })),
    });
  }
  const admin: Omit<Entry, "id">[] = [];
  if (me.data?.manage_rights === true) {
    admin.push({
      key: "roles",
      label: t("nav.roles"),
      link: (
        <Link to="/roles" className={itemClass} activeProps={{ className: activeClass }}>
          {t("nav.roles")}
        </Link>
      ),
    });
  }
  if (me.data?.customize === true) {
    admin.push({
      key: "customize",
      label: t("nav.customize"),
      link: (
        <Link to="/customize" className={itemClass} activeProps={{ className: activeClass }}>
          {t("nav.customize")}
        </Link>
      ),
    });
  }
  if (admin.length > 0) {
    areas.push({ id: "admin", label: t("nav.admin"), icon: Shield, entries: admin });
  }
  return areas.map((a) => ({
    ...a,
    entries: a.entries.map((e) => ({ ...e, id: `${a.id}/${e.key}` })),
  }));
}

// arrange puts the areas in the order of the person (UI37, UI-Plan E50); an area the order does
// not name yet (a package installed later) comes after the one it usually follows.
export function arrange(areas: Area[], order: string[]): Area[] {
  const byId = new Map(areas.map((a) => [a.id, a]));
  const out = [...new Set(order)].flatMap((id) => byId.get(id) ?? []);
  areas.forEach((a, i) => {
    if (out.includes(a)) {
      return;
    }
    let at = 0;
    for (let k = i - 1; k >= 0; k--) {
      const j = out.indexOf(areas[k]!);
      if (j >= 0) {
        at = j + 1;
        break;
      }
    }
    out.splice(at, 0, a);
  });
  return out;
}

// pinned are the favourites that still lead somewhere the session may go, in their order.
export function pinned(areas: Area[], favorites: string[]): Entry[] {
  const byId = new Map(areas.flatMap((a) => a.entries.map((e) => [e.id, e] as const)));
  return favorites.flatMap((id) => byId.get(id) ?? []);
}

export type Arrangement = { nav_order?: string[]; nav_hidden?: string[]; favorites?: string[] };

// useArrangement reads the arrangement of the navigation and keeps a change at once: shown
// before the server answers, so a star reacts without delay.
export function useArrangement() {
  const queryClient = useQueryClient();
  const settings = useQuery(mySettingsQuery);
  const s = settings.data;
  const save = (change: Arrangement) => {
    queryClient.setQueryData(mySettingsQuery.queryKey, (old) =>
      old ? { ...old, ...change } : old,
    );
    void api
      .setMySettings(change)
      .then((saved) => queryClient.setQueryData(mySettingsQuery.queryKey, saved))
      .catch(() => queryClient.invalidateQueries({ queryKey: mySettingsQuery.queryKey }));
  };
  return {
    order: s?.nav_order ?? [],
    hidden: s?.nav_hidden ?? [],
    favorites: s?.favorites ?? [],
    save,
  };
}

// Pins are the favourites with the way to change them, for the stars of the entries.
type Pins = { has: (id: string) => boolean; toggle: (id: string) => void };

export function Nav() {
  const { t } = useTranslation();
  const me = useQuery(meQuery);
  const settings = useQuery(mySettingsQuery);
  const approvals = useQuery({ ...myApprovalsQuery, enabled: me.data !== undefined });
  const types = useQuery(typesQuery);
  const keep = usePersist(true);
  const all = useAreas();
  const arrangement = useArrangement();
  const { favorites, save } = arrangement;
  const favoriteEntries = pinned(all, favorites);
  const hidden = new Set(arrangement.hidden);
  const areas: Area[] = [
    ...(favoriteEntries.length > 0
      ? [{ id: "favorites", label: t("nav.favorites"), icon: Star, entries: favoriteEntries }]
      : []),
    ...arrange(all, arrangement.order).filter((a) => !hidden.has(a.id)),
  ];
  const pins: Pins = {
    has: (id) => favorites.includes(id),
    toggle: (id) =>
      save({
        favorites: favorites.includes(id) ? favorites.filter((f) => f !== id) : [...favorites, id],
      }),
  };
  const account = me.data?.id ?? "";
  const [open, setOpen] = useState<Record<string, boolean>>({});
  useEffect(() => {
    if (account) {
      setOpen(readOpen(account));
    }
  }, [account]);
  const desktop = useDesktop();
  const [narrow, setNarrow] = useState(false);
  const own = settings.data?.nav?.value;
  useEffect(() => {
    setNarrow(own === "narrow");
  }, [own]);
  const width = (value: "wide" | "narrow") => {
    setNarrow(value === "narrow");
    keep({ nav: value });
  };
  const toggle = (id: string) => {
    const next = { ...open, [id]: !(open[id] ?? true) };
    setOpen(next);
    try {
      localStorage.setItem(openKey(account), JSON.stringify(next));
    } catch {
      // Storage may be blocked; the area still opens and closes for this page.
    }
  };
  const count = approvals.data?.length;

  const wide = (
    <nav
      aria-label={t("nav.types")}
      className="flex w-full flex-col border-b bg-surface-subtle md:min-h-0 md:w-60 md:flex-none md:border-r md:border-b-0"
    >
      <div className="flex flex-col gap-3 px-2.5 pt-3 pb-4 md:min-h-0 md:flex-1 md:overflow-y-auto">
        <ul className="flex flex-col gap-px">
          <li>
            <Link
              to="/"
              className={headClass}
              activeOptions={{ exact: true }}
              activeProps={{ className: activeClass }}
            >
              <House aria-hidden="true" className="size-4 flex-none text-foreground-2" />
              {t("nav.start")}
            </Link>
          </li>
          <li>
            {/* Zu genehmigen with the open questions (WF6), for everyone signed in. */}
            <Link to="/approvals" className={headClass} activeProps={{ className: activeClass }}>
              <ListChecks aria-hidden="true" className="size-4 flex-none text-foreground-2" />
              <span className="flex-1">{t("nav.approvals")}</span>
              {count !== undefined && (
                <span className="font-mono text-xs text-foreground-2">{count}</span>
              )}
            </Link>
          </li>
        </ul>
        {areas.map((area) => (
          <AreaBlock
            key={area.id}
            area={area}
            open={open[area.id] ?? true}
            onToggle={() => toggle(area.id)}
            pins={pins}
          />
        ))}
        {types.isError && <p className="text-sm text-destructive">{t("nav.typesError")}</p>}
      </div>
      <div className="hidden flex-none border-t px-2.5 py-2 md:block">
        <button
          type="button"
          onClick={() => width("narrow")}
          className="flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-left text-sm text-foreground-2 hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <PanelLeftClose aria-hidden="true" className="size-4" />
          {t("nav.collapse")}
        </button>
      </div>
    </nav>
  );
  // One navigation at a time: below 768 px always the wide one (UI30, until UI40).
  if (!narrow || !desktop) {
    return wide;
  }
  return <Rail areas={areas} approvals={count} onWide={() => width("wide")} pins={pins} />;
}

// EntryRow is an entry with its star (UI37): shown when pointed at or focused, always when
// pinned.
function EntryRow({ entry, pins }: { entry: Entry; pins: Pins }) {
  const { t } = useTranslation();
  const on = pins.has(entry.id);
  const label = t(on ? "nav.unpin" : "nav.pin", { name: entry.label });
  return (
    <li className="group/entry flex items-center gap-0.5 [&>a]:min-w-0 [&>a]:flex-1">
      {entry.link}
      <button
        type="button"
        aria-pressed={on}
        aria-label={label}
        title={label}
        onClick={() => pins.toggle(entry.id)}
        className={`inline-grid size-7 flex-none place-items-center rounded-md text-muted-foreground hover:bg-surface-active hover:text-foreground focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
          on ? "" : "md:opacity-0 md:group-hover/entry:opacity-100"
        }`}
      >
        <Star aria-hidden="true" className="size-3.5" fill={on ? "currentColor" : "none"} />
      </button>
    </li>
  );
}

// AreaBlock is one area of the wide bar: its head opens and closes it.
function AreaBlock({
  area,
  open,
  onToggle,
  pins,
}: {
  area: Area;
  open: boolean;
  onToggle: () => void;
  pins: Pins;
}) {
  const id = useId();
  const Icon = area.icon;
  return (
    <div className="flex flex-col gap-px">
      <div className="flex items-center gap-0.5">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={`${id}-list`}
          onClick={onToggle}
          className="flex h-9 min-w-0 flex-1 items-center gap-2.5 rounded-md pr-1.5 pl-2 text-left hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Icon aria-hidden="true" className="size-4 flex-none text-foreground-2" />
          <span className="min-w-0 flex-1 truncate text-[13.5px] font-semibold">{area.label}</span>
          {!open && (
            <span aria-hidden="true" className="font-mono text-xs text-muted-foreground">
              {area.entries.length}
            </span>
          )}
          <ChevronDown
            aria-hidden="true"
            className={`size-4 flex-none text-foreground-2 transition-transform ${open ? "" : "-rotate-90"}`}
          />
        </button>
        {area.add}
      </div>
      {open && (
        <ul id={`${id}-list`} className="flex flex-col gap-px">
          {area.entries.map((e) => (
            <EntryRow key={e.id} entry={e} pins={pins} />
          ))}
        </ul>
      )}
    </div>
  );
}

const railButton =
  "relative inline-grid size-10 flex-none place-items-center rounded-lg text-foreground-2 hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

// Rail is the narrow bar of 60 px: symbols only; an area shows its entries beside it.
function Rail({
  areas,
  approvals,
  onWide,
  pins,
}: {
  areas: Area[];
  approvals: number | undefined;
  onWide: () => void;
  pins: Pins;
}) {
  const { t } = useTranslation();
  return (
    <nav
      aria-label={t("nav.types")}
      className="relative flex min-h-0 w-[60px] flex-none flex-col border-r bg-surface-subtle"
    >
      <div className="flex min-h-0 flex-1 flex-col items-center gap-1 overflow-y-auto px-2.5 py-3 [scrollbar-width:none]">
        <Link
          to="/"
          aria-label={t("nav.start")}
          title={t("nav.start")}
          className={railButton}
          activeOptions={{ exact: true }}
          activeProps={{ className: "bg-surface-active text-foreground" }}
        >
          <House aria-hidden="true" className="size-5" />
        </Link>
        <Link
          to="/approvals"
          aria-label={t("nav.approvals")}
          title={t("nav.approvals")}
          className={railButton}
          activeProps={{ className: "bg-surface-active text-foreground" }}
        >
          <ListChecks aria-hidden="true" className="size-5" />
          {approvals !== undefined && approvals > 0 && (
            <span
              aria-hidden="true"
              className="absolute top-0.5 right-0 h-4 min-w-4 rounded-full border bg-card px-1 font-mono text-[10px] leading-3.5 font-medium text-foreground"
            >
              {approvals}
            </span>
          )}
        </Link>
        {areas.map((area) => (
          <RailArea key={area.id} area={area} pins={pins} />
        ))}
      </div>
      <div className="flex flex-none justify-center border-t px-2.5 py-2">
        <button
          type="button"
          aria-label={t("nav.expand")}
          title={t("nav.expand")}
          onClick={onWide}
          className="inline-grid h-8 w-10 place-items-center rounded-md text-foreground-2 hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <PanelLeftOpen aria-hidden="true" className="size-4" />
        </button>
      </div>
    </nav>
  );
}

// RailArea is the symbol of an area; a click shows its entries beside the bar, Esc closes them,
// and so does going to one of them.
function RailArea({ area, pins }: { area: Area; pins: Pins }) {
  const pop = usePopover();
  const Icon = area.icon;
  const [top, setTop] = useState(64);
  const href = useLocation({ select: (l) => l.href });
  const { close } = pop;
  useEffect(() => {
    close(false);
  }, [href, close]);
  return (
    <>
      <button
        ref={pop.trigger}
        type="button"
        aria-label={area.label}
        title={area.label}
        aria-expanded={pop.open}
        onClick={(e) => {
          // Beside the symbol, and so that a list of some entries still fits below.
          setTop(
            Math.max(
              64,
              Math.min(e.currentTarget.getBoundingClientRect().top - 8, window.innerHeight - 280),
            ),
          );
          pop.toggle();
        }}
        className={`${railButton} ${pop.open ? "bg-accent-soft text-foreground" : ""}`}
      >
        <Icon aria-hidden="true" className="size-5" />
      </button>
      {pop.open && (
        <div
          ref={pop.panel}
          role="group"
          aria-label={area.label}
          className="fixed left-[66px] z-40 max-h-[70vh] w-62 overflow-y-auto rounded-md border border-input bg-card p-2 shadow-lg"
          style={{ top }}
        >
          <div className="flex items-center justify-between px-2 pt-1 pb-1.5">
            <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
              {area.label}
            </p>
            {area.add}
          </div>
          <ul className="flex flex-col gap-px [&_a]:pl-2">
            {area.entries.map((e) => (
              <EntryRow key={e.id} entry={e} pins={pins} />
            ))}
          </ul>
        </div>
      )}
    </>
  );
}
