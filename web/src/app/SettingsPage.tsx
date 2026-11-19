// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Settings (UI9, Hauptplan E131; UI38, UI plan E41, E49). "Meine Einstellungen" in tabs:
// language and time, appearance (mode as three cards, accent from six colours, row height,
// start page), presets per type for creating, substitutes. Every change applies and is kept at
// once; "Auf Vorgabe zurücksetzen" takes the own values of the open tab away, so that the unit's
// or the default apply. "Globale Einstellungen" (GlobalSettings.tsx): the settings at a node of
// the organisation tree, for those who hold manage_settings there (Kern-Plan E59; SD20).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, Check, Star } from "lucide-react";
import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  dashboardsQuery,
  meQuery,
  myBrandingQuery,
  mySettingsQuery,
  qualifiedName,
  typesQuery,
  type MySettings,
  type SettingsChange,
} from "@/api/client";
import { defaultName } from "@/app/Brand";
import { LookPreview } from "@/app/LookPreview";
import { arrange, pinned, useAreas } from "@/app/Nav";
import { languages } from "@/i18n";
import { FieldValue } from "@/render/Batch";
import { fieldLabel, isCalculated } from "@/render/fields";
import { Substitutes } from "@/render/Substitutes";
import { accentColors, accents } from "@/ui/accent";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Label } from "@/ui/label";
import { Select } from "@/ui/select";
import { toast } from "@/ui/toast";

// timeZones are the zones the browser knows; Europe/Berlin first.
export function timeZones(): string[] {
  const all =
    typeof Intl.supportedValuesOf === "function"
      ? Intl.supportedValuesOf("timeZone")
      : ["Europe/Berlin", "Europe/London", "UTC"];
  return ["Europe/Berlin", ...all.filter((z) => z !== "Europe/Berlin")];
}

function useSource() {
  const { t } = useTranslation();
  return (source: string) =>
    source === "own"
      ? t("settings.source.own")
      : source === "default"
        ? t("settings.source.default")
        : t("settings.source.node", { node: source });
}

type Key = "language" | "time_zone" | "color_scheme" | "density" | "start_dashboard" | "accent";

// useSave keeps a change at once and tells how it went (E49), as a message at the bottom right
// (UI60): the page does not move.
function useSave() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const save = useMutation({
    mutationFn: (change: SettingsChange) => api.setMySettings(change),
    onMutate: (change) => {
      // The lists of the navigation show at once (UI37), so that a second click builds on the
      // first; the server's answer replaces them.
      const lists = Object.fromEntries(
        (["nav_order", "nav_hidden", "favorites"] as const)
          .filter((k) => k in change)
          .map((k) => [k, change[k] ?? []]),
      );
      if (Object.keys(lists).length > 0) {
        queryClient.setQueryData(mySettingsQuery.queryKey, (old) =>
          old ? { ...old, ...lists } : old,
        );
      }
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(mySettingsQuery.queryKey, saved);
      toast.success(t("form.saved"));
    },
    onError: (err) => {
      void queryClient.invalidateQueries({ queryKey: mySettingsQuery.queryKey });
      toast.error(
        t("settings.error", { detail: err instanceof Error ? err.message : String(err) }),
      );
    },
  });
  return {
    keep: (change: SettingsChange) => save.mutate(change),
    pending: save.isPending,
  };
}

// inForce says which value holds now and where it comes from.
function InForce({
  settings,
  name,
  show,
}: {
  settings: MySettings;
  name: Key;
  show?: (v: string) => string;
}) {
  const { t } = useTranslation();
  const source = useSource();
  const now = settings[name];
  return (
    <p className="text-sm text-muted-foreground">
      {now.value === ""
        ? t("settings.inForceNone")
        : t("settings.inForce", {
            value: show ? show(now.value) : now.value,
            source: source(now.source),
          })}
    </p>
  );
}

function Choice({
  name,
  settings,
  onChange,
  options,
  show,
}: {
  name: Key;
  settings: MySettings;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
  // show names a value in force, such as the name of a dashboard (UI13).
  show?: (v: string) => string;
}) {
  const { t } = useTranslation();
  const id = useId();
  const own = settings[name].source === "own" ? settings[name].value : "";
  return (
    <div className="flex flex-col gap-1">
      <Label htmlFor={id}>{t(`settings.${name}`)}</Label>
      <Select
        id={id}
        value={own}
        onChange={onChange}
        options={[{ value: "", label: t("settings.inherit") }, ...options]}
      />
      <InForce settings={settings} name={name} show={show} />
    </div>
  );
}

// Section is a titled part of a tab with an optional hint.
function Section({ title, hint, children }: { title: string; hint?: string; children: ReactNode }) {
  const id = useId();
  return (
    <section aria-labelledby={id} className="flex flex-col gap-3 rounded-md border bg-card p-4">
      <div>
        <h2 id={id} className="text-[15px] font-semibold">
          {title}
        </h2>
        {hint && <p className="text-sm text-muted-foreground">{hint}</p>}
      </div>
      {children}
    </section>
  );
}

// Move is the pair of buttons that moves a row up or down in a list.
function Move({
  up,
  down,
  at,
  count,
  onMove,
}: {
  up: string;
  down: string;
  at: number;
  count: number;
  onMove: (from: number, to: number) => void;
}) {
  const button =
    "inline-grid size-7 flex-none place-items-center rounded-md text-foreground-2 hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:text-muted-foreground/50 disabled:hover:bg-transparent";
  return (
    <>
      <button
        type="button"
        aria-label={up}
        title={up}
        disabled={at === 0}
        onClick={() => onMove(at, at - 1)}
        className={button}
      >
        <ArrowUp aria-hidden="true" className="size-4" />
      </button>
      <button
        type="button"
        aria-label={down}
        title={down}
        disabled={at === count - 1}
        onClick={() => onMove(at, at + 1)}
        className={button}
      >
        <ArrowDown aria-hidden="true" className="size-4" />
      </button>
    </>
  );
}

// moved is the list with the item at from put at to.
function moved<T>(list: T[], from: number, to: number): T[] {
  const out = [...list];
  const [item] = out.splice(from, 1);
  out.splice(to, 0, item!);
  return out;
}

// Navigation is the part of Darstellung for the bar (UI37, UI-Plan E50): its width, the order
// and visibility of the areas, and the favourites in their order.
function Navigation({
  settings,
  keep,
}: {
  settings: MySettings;
  keep: (c: SettingsChange) => void;
}) {
  const { t } = useTranslation();
  const all = useAreas();
  const areas = arrange(all, settings.nav_order);
  const hidden = new Set(settings.nav_hidden);
  const favorites = pinned(all, settings.favorites);
  const areaOf = new Map(all.flatMap((a) => a.entries.map((e) => [e.id, a.label] as const)));
  const width = settings.nav.value;
  const row = "flex min-h-11 items-center gap-2.5 border-b py-1 pr-2 pl-3 last:border-b-0";
  return (
    <Section title={t("settings.nav")} hint={t("settings.navHint")}>
      <div
        role="group"
        aria-label={t("settings.navWidth")}
        className="inline-flex w-fit gap-0.5 rounded-md bg-muted p-0.5"
      >
        {(["wide", "narrow"] as const).map((w) => (
          <button
            key={w}
            type="button"
            aria-pressed={width === w}
            onClick={() => keep({ nav: w })}
            className={`h-8 rounded-sm px-3.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
              width === w
                ? "bg-segment-active font-medium text-foreground shadow-sm"
                : "text-foreground-2"
            }`}
          >
            {t(`settings.navWidths.${w}`)}
          </button>
        ))}
      </div>
      <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {t("settings.areas")}
      </h3>
      <ul className="flex flex-col rounded-md border">
        {areas.map((a, i) => {
          const Icon = a.icon;
          const shown = !hidden.has(a.id);
          return (
            <li key={a.id} className={row}>
              <Icon aria-hidden="true" className="size-4 flex-none text-foreground-2" />
              <span
                className={`min-w-0 flex-1 truncate text-sm font-medium ${shown ? "" : "text-muted-foreground"}`}
              >
                {a.label}
              </span>
              <span className="text-sm text-muted-foreground">
                {t("settings.entries", { count: a.entries.length })}
              </span>
              <Move
                up={t("settings.up", { name: a.label })}
                down={t("settings.down", { name: a.label })}
                at={i}
                count={areas.length}
                onMove={(from, to) => keep({ nav_order: moved(areas, from, to).map((x) => x.id) })}
              />
              <label className="flex w-24 items-center gap-1.5 text-sm text-foreground-2">
                <input
                  type="checkbox"
                  className="size-4 accent-[var(--accent)]"
                  aria-label={t("settings.show", { name: a.label })}
                  checked={shown}
                  onChange={() =>
                    keep({
                      nav_hidden: shown
                        ? [...settings.nav_hidden, a.id]
                        : settings.nav_hidden.filter((h) => h !== a.id),
                    })
                  }
                />
                <span aria-hidden="true">{t("settings.shown")}</span>
              </label>
            </li>
          );
        })}
      </ul>
      <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {t("nav.favorites")}
      </h3>
      {favorites.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("settings.noFavorites")}</p>
      ) : (
        <ul className="flex flex-col rounded-md border">
          {favorites.map((e, i) => (
            <li key={e.id} className={row}>
              <Star aria-hidden="true" className="size-4 flex-none text-foreground-2" />
              <span className="min-w-0 flex-1 truncate text-sm font-medium">{e.label}</span>
              <span className="truncate text-sm text-muted-foreground">{areaOf.get(e.id)}</span>
              <Move
                up={t("settings.favoriteUp", { name: e.label })}
                down={t("settings.favoriteDown", { name: e.label })}
                at={i}
                count={favorites.length}
                onMove={(from, to) => {
                  // Favourites that lead nowhere now stay kept, after the others.
                  const shownIds = favorites.map((f) => f.id);
                  const rest = settings.favorites.filter((f) => !shownIds.includes(f));
                  keep({ favorites: [...moved(shownIds, from, to), ...rest] });
                }}
              />
              <Button
                variant="ghost"
                size="sm"
                aria-label={t("settings.unpinFavorite", { name: e.label })}
                onClick={() => keep({ favorites: settings.favorites.filter((f) => f !== e.id) })}
              >
                {t("settings.unpin")}
              </Button>
            </li>
          ))}
        </ul>
      )}
      <p className="text-sm text-muted-foreground">{t("settings.favoritesHint")}</p>
    </Section>
  );
}

const modePreview: Record<string, string> = {
  light: "linear-gradient(#FFFFFF, #FFFFFF) 10px 10px / 46% 12px no-repeat, #F6F6F3",
  dark: "linear-gradient(#2C3035, #2C3035) 10px 10px / 46% 12px no-repeat, #111315",
  system: "linear-gradient(90deg, #F6F6F3 50%, #111315 50%)",
};

// Appearance is the tab Darstellung (UI38): mode, accent, row height and the start page.
function Appearance({
  settings,
  keep,
  workplace,
}: {
  settings: MySettings;
  keep: (c: SettingsChange) => void;
  workplace: boolean;
}) {
  const { t } = useTranslation();
  const branding = useQuery(myBrandingQuery);
  const dashboards = useQuery({ ...dashboardsQuery, enabled: workplace });
  const boards = (dashboards.data?.items ?? []).map((b) => ({
    value: String(b["id"]),
    label: String(b["name"]),
  }));
  const showBoard = (v: string) =>
    v === "none" ? t("settings.startNone") : (boards.find((b) => b.value === v)?.label ?? v);
  const card = (on: boolean) =>
    `flex flex-1 flex-col gap-2 rounded-md p-2.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
      on ? "border-2 border-accent" : "border-2 border-border hover:border-input"
    }`;
  const allowed = settings.allowed;
  const scheme = settings.color_scheme.value;
  const accent = settings.accent?.value ?? "petrol";
  const density = settings.density.value;
  // Beside the choices a small picture of the interface that follows them at once (UI55).
  return (
    <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <div className="flex min-w-0 flex-col gap-4">
        {allowed.color_scheme && (
          <Section title={t("settings.mode")}>
            <div className="flex flex-col gap-3 sm:flex-row">
              {(["light", "dark", "system"] as const).map((m) => (
                <button
                  key={m}
                  type="button"
                  aria-pressed={scheme === m}
                  onClick={() => keep({ color_scheme: m })}
                  className={card(scheme === m)}
                >
                  <span
                    aria-hidden="true"
                    className="block h-14 rounded-sm border"
                    style={{ background: modePreview[m] }}
                  />
                  <span className={`text-sm ${scheme === m ? "font-semibold" : ""}`}>
                    {t(`colorScheme.${m}`)}
                  </span>
                </button>
              ))}
            </div>
            <InForce settings={settings} name="color_scheme" show={(v) => t(`colorScheme.${v}`)} />
          </Section>
        )}
        {allowed.accent && (
          <Section title={t("settings.accent")} hint={t("settings.accentHint")}>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
              {accents.map((a) => (
                <button
                  key={a}
                  type="button"
                  aria-pressed={accent === a}
                  onClick={() => keep({ accent: a })}
                  className={`flex h-11 items-center gap-2.5 rounded-md border px-2.5 text-left text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                    accent === a
                      ? "border-accent bg-accent-soft font-medium"
                      : "hover:bg-surface-active"
                  }`}
                >
                  <span
                    aria-hidden="true"
                    className="inline-grid size-6 flex-none place-items-center rounded-full text-white ring-1 ring-border-strong/40"
                    style={{ background: accentColors[a] }}
                  >
                    {accent === a && <Check className="size-3.5" />}
                  </span>
                  {t(`accent.${a}`)}
                </button>
              ))}
            </div>
            <InForce
              settings={settings}
              name="accent"
              show={(v) =>
                v.startsWith("#") ? t("settings.orgAccent", { color: v }) : t(`accent.${v}`)
              }
            />
          </Section>
        )}
        {allowed.density && (
          <Section title={t("settings.density")} hint={t("settings.densityHint")}>
            <div
              role="group"
              aria-label={t("settings.density")}
              className="inline-flex w-fit gap-0.5 rounded-md bg-muted p-0.5"
            >
              {(["compact", "normal"] as const).map((d) => (
                <button
                  key={d}
                  type="button"
                  aria-pressed={density === d}
                  onClick={() => keep({ density: d })}
                  className={`h-8 rounded-sm px-3.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                    density === d
                      ? "bg-segment-active font-medium text-foreground shadow-sm"
                      : "text-foreground-2"
                  }`}
                >
                  {t(`list.densities.${d}`)}
                </button>
              ))}
            </div>
          </Section>
        )}
        {/* Single keys such as / and N, which the person may turn off (UI52, UI-Plan E55). */}
        <Section title={t("settings.shortcuts")} hint={t("settings.shortcutsHint")}>
          <div
            role="group"
            aria-label={t("settings.shortcuts")}
            className="inline-flex w-fit gap-0.5 rounded-md bg-muted p-0.5"
          >
            {(["on", "off"] as const).map((v) => {
              const on = (settings.shortcuts?.value ?? "on") === v;
              return (
                <button
                  key={v}
                  type="button"
                  aria-pressed={on}
                  onClick={() => keep({ shortcuts: v })}
                  className={`h-8 rounded-sm px-3.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                    on
                      ? "bg-segment-active font-medium text-foreground shadow-sm"
                      : "text-foreground-2"
                  }`}
                >
                  {t(`settings.${v}`)}
                </button>
              );
            })}
          </div>
        </Section>
        {workplace && allowed.nav && <Navigation settings={settings} keep={keep} />}
        {!(allowed.color_scheme && allowed.accent && allowed.density && allowed.nav) && (
          <p className="text-sm text-muted-foreground">{t("settings.byOrganisation")}</p>
        )}
        {workplace && (
          <div className="rounded-md border bg-card p-4">
            <Choice
              name="start_dashboard"
              settings={settings}
              onChange={(v) => keep({ start_dashboard: v || null })}
              options={[...boards, { value: "none", label: t("settings.startNone") }]}
              show={showBoard}
            />
          </div>
        )}
      </div>
      <div className="flex flex-col gap-1.5 lg:sticky lg:top-4">
        <span className="text-xs font-medium tracking-wide text-foreground-2 uppercase">
          {t("appearance.preview")}
        </span>
        <LookPreview
          scheme={scheme}
          density={density}
          nav={settings.nav.value}
          accent={accent}
          primary={branding.data?.primary_accent ?? false}
          name={branding.data?.name ?? defaultName}
          logoLight={branding.data?.logo_light ?? null}
          logoDark={branding.data?.logo_dark ?? null}
        />
      </div>
    </div>
  );
}

// Presets are the values per type for creating; a change is kept a moment after the last one.
function Presets({ settings }: { settings: MySettings }) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const queryClient = useQueryClient();
  const types = useQuery(typesQuery);
  const [typeName, setTypeName] = useState("");
  const [rows, setRows] = useState<{ field: string; value: unknown }[]>([]);
  const [dirty, setDirty] = useState(false);
  const type = types.data?.find((x) => qualifiedName(x) === typeName);
  const fields = (type?.fields ?? []).filter((f) => !isCalculated(f) && f.format !== "rich");
  const pick = (name: string) => {
    setTypeName(name);
    setDirty(false);
    setRows(
      Object.entries(settings.defaults[name] ?? {}).map(([field, value]) => ({
        field,
        value:
          typeof value === "boolean" || typeof value === "number" ? value : String(value ?? ""),
      })),
    );
  };
  const edit = (next: { field: string; value: unknown }[]) => {
    setRows(next);
    setDirty(true);
  };
  const save = useMutation({
    mutationFn: () => {
      const values: Record<string, unknown> = {};
      for (const r of rows) {
        const f = fields.find((x) => x.name === r.field);
        if (!f || r.value === "" || r.value === undefined) {
          continue;
        }
        values[r.field] =
          f.type === "number"
            ? Number(r.value)
            : f.type === "boolean"
              ? r.value === true || r.value === "true"
              : r.value;
      }
      return api.setMySettings({
        defaults: { [typeName]: Object.keys(values).length > 0 ? values : null },
      });
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(mySettingsQuery.queryKey, saved);
      setDirty(false);
      toast.success(t("form.saved"));
    },
    onError: (err) =>
      toast.error(
        t("settings.error", { detail: err instanceof Error ? err.message : String(err) }),
      ),
  });
  const { mutate } = save;
  useEffect(() => {
    if (!dirty || !typeName) {
      return;
    }
    const timer = setTimeout(() => mutate(), 600);
    return () => clearTimeout(timer);
  }, [rows, dirty, typeName, mutate]);
  return (
    <Section title={t("settings.presets")} hint={t("settings.presetsHint")}>
      <div className="flex flex-col gap-1">
        <Label htmlFor={`${id}-type`}>{t("settings.type")}</Label>
        <Select
          id={`${id}-type`}
          value={typeName}
          onChange={pick}
          options={[
            { value: "", label: t("form.none") },
            ...(types.data ?? []).map((x) => ({
              value: qualifiedName(x),
              label: labelOf(x.label, i18n.language, qualifiedName(x)),
            })),
          ]}
        />
      </div>
      {type && (
        <>
          <ul className="flex flex-col gap-2">
            {rows.map((r, i) => {
              const f = fields.find((x) => x.name === r.field);
              return (
                <li
                  key={i}
                  className="flex flex-wrap items-start gap-2"
                  aria-label={t("settings.preset", { n: i + 1 })}
                >
                  <Select
                    label={t("batch.field")}
                    value={r.field}
                    onChange={(v) =>
                      edit(rows.map((x, j) => (j === i ? { field: v, value: "" } : x)))
                    }
                    options={fields.map((x) => ({
                      value: x.name,
                      label: fieldLabel(x, i18n.language),
                    }))}
                  />
                  {f && (
                    <FieldValue
                      key={r.field}
                      id={`${id}-v${i}`}
                      field={f}
                      value={r.value}
                      onChange={(v) => edit(rows.map((x, j) => (j === i ? { ...x, value: v } : x)))}
                    />
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-10"
                    onClick={() => edit(rows.filter((_, j) => j !== i))}
                  >
                    {t("filter.remove")}
                  </Button>
                </li>
              );
            })}
          </ul>
          <Button
            variant="outline"
            size="sm"
            className="w-fit"
            disabled={fields.length === 0}
            onClick={() => setRows([...rows, { field: fields[0]!.name, value: "" }])}
          >
            {t("settings.addPreset")}
          </Button>
        </>
      )}
    </Section>
  );
}

type Tab = "language" | "appearance" | "presets" | "substitutes";

export function MySettingsPage() {
  const { t } = useTranslation();
  const id = useId();
  // In the portal (UI10) presets, the start page and the global settings are of no use.
  const me = useQuery(meQuery);
  const settings = useQuery(mySettingsQuery);
  const workplace = me.data?.workplace !== false;
  const { keep } = useSave();
  const [tab, setTab] = useState<Tab>("language");
  const tabs = useRef<(HTMLButtonElement | null)[]>([]);
  if (!settings.data) {
    return settings.isError ? (
      <Alert variant="destructive">{t("settings.loadError")}</Alert>
    ) : (
      <p>{t("list.loading")}</p>
    );
  }
  const s = settings.data;
  const list: Tab[] = [
    "language",
    "appearance",
    ...(workplace ? (["presets"] as const) : []),
    ...(me.data?.person_id ? (["substitutes"] as const) : []),
  ];
  const reset: Partial<Record<Tab, () => void>> = {
    language: () => keep({ language: null, time_zone: null }),
    appearance: () =>
      keep({
        // Only what the person may set; the rest is the organisation's (UI39).
        ...(s.allowed.color_scheme ? { color_scheme: null } : {}),
        ...(s.allowed.accent ? { accent: null } : {}),
        ...(s.allowed.density ? { density: null } : {}),
        shortcuts: null,
        ...(workplace
          ? {
              start_dashboard: null,
              ...(s.allowed.nav
                ? { nav: null, nav_order: null, nav_hidden: null, favorites: null }
                : {}),
            }
          : {}),
      }),
    presets: () =>
      keep({ defaults: Object.fromEntries(Object.keys(s.defaults).map((k) => [k, null])) }),
  };
  // The arrow keys move between the tabs (WAI-ARIA tabs).
  const onKeys = (e: KeyboardEvent<HTMLButtonElement>) => {
    const at = list.indexOf(tab);
    const next =
      e.key === "ArrowRight"
        ? (at + 1) % list.length
        : e.key === "ArrowLeft"
          ? (at - 1 + list.length) % list.length
          : -1;
    if (next >= 0) {
      e.preventDefault();
      setTab(list[next]!);
      tabs.current[next]?.focus();
    }
  };
  return (
    <div className={`flex ${tab === "appearance" ? "max-w-5xl" : "max-w-3xl"} flex-col gap-5`}>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">{t("settings.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("settings.intro")}</p>
        </div>
        <div className="flex items-center gap-3">
          {reset[tab] && (
            <Button variant="outline" onClick={reset[tab]}>
              {t("settings.reset")}
            </Button>
          )}
        </div>
      </div>
      <div
        role="tablist"
        aria-label={t("settings.tabs.label")}
        className="flex flex-wrap gap-0.5 border-b"
      >
        {list.map((x, i) => (
          <button
            key={x}
            ref={(el) => {
              tabs.current[i] = el;
            }}
            type="button"
            role="tab"
            id={`${id}-tab-${x}`}
            aria-selected={tab === x}
            aria-controls={`${id}-panel`}
            tabIndex={tab === x ? 0 : -1}
            onClick={() => setTab(x)}
            onKeyDown={onKeys}
            className={`-mb-px border-b-2 px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
              tab === x
                ? "border-accent font-semibold text-foreground"
                : "border-transparent text-foreground-2 hover:text-foreground"
            }`}
          >
            {t(`settings.tabs.${x}`)}
          </button>
        ))}
      </div>
      <div
        role="tabpanel"
        id={`${id}-panel`}
        aria-labelledby={`${id}-tab-${tab}`}
        className="flex flex-col gap-4"
      >
        {tab === "language" && (
          <Section title={t("settings.tabs.language")}>
            <Choice
              name="language"
              settings={s}
              onChange={(v) => keep({ language: (v || null) as "de" | "en" | null })}
              options={Object.keys(languages).map((l) => ({ value: l, label: t(`language.${l}`) }))}
            />
            <Choice
              name="time_zone"
              settings={s}
              onChange={(v) => keep({ time_zone: v || null })}
              options={timeZones().map((z) => ({ value: z, label: z }))}
            />
          </Section>
        )}
        {tab === "appearance" && <Appearance settings={s} keep={keep} workplace={workplace} />}
        {tab === "presets" && <Presets settings={s} />}
        {tab === "substitutes" && me.data?.person_id && (
          <Substitutes personId={me.data.person_id} />
        )}
      </div>
      {workplace && (
        <p className="text-sm">
          <Link to="/settings/global" className="underline">
            {t("settings.global")}
          </Link>
        </p>
      )}
    </div>
  );
}
