// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The appearance of the organisation under Verwaltung (UI39, UI plan E41, E45, E51): name and
// logo, the accent colour with its contrast, the main action in the accent, what each person may
// set herself and the defaults. It holds for the whole tenant (its node, the root without
// tenants), is seen in a preview and kept with "Speichern" (E51), not at once.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState, type CSSProperties, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { api, ApiError, labelOf, meQuery, myBrandingQuery } from "@/api/client";
import { defaultName } from "@/app/Brand";
import { LookPreview } from "@/app/LookPreview";
import { accentTokens, contrast } from "@/ui/accent";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";

const selectClass = "h-10 rounded-md border border-input bg-background px-3 text-sm";
const hexRe = /^#[0-9a-fA-F]{6}$/;
const variants = ["light", "dark"] as const;
type Variant = (typeof variants)[number];
const own = ["color_scheme", "accent", "density", "nav"] as const;
type Own = (typeof own)[number];

// Form is what the page edits; logos are a new file, "remove", or unchanged (undefined).
type Form = {
  name: string;
  accent: string;
  primary: boolean;
  allowed: Record<Own, boolean>;
  scheme: string;
  density: string;
  nav: string;
};

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

export function AppearancePage() {
  const { t, i18n } = useTranslation();
  const id = useId();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  const node = me.data?.tenant ?? "root";
  const settings = useQuery({
    queryKey: ["org", "settings", node],
    queryFn: () => api.orgSettings(node),
    enabled: me.data?.appearance === true,
  });
  const branding = useQuery(myBrandingQuery);
  const [form, setForm] = useState<Form | null>(null);
  const [logos, setLogos] = useState<Partial<Record<Variant, File | "remove">>>({});
  const [previews, setPreviews] = useState<Partial<Record<Variant, string>>>({});
  const [message, setMessage] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!settings.data || form) {
      return;
    }
    const value = (key: string) => settings.data.find((s) => s.key === key)?.value;
    const bool = (key: string, fallback: boolean) => {
      const v = value(key);
      return typeof v === "boolean" ? v : fallback;
    };
    const text = (key: string, fallback: string) => {
      const v = value(key);
      return typeof v === "string" && v !== "" ? v : fallback;
    };
    setForm({
      name: text("ui.app_name", defaultName),
      accent: text("ui.accent", "#0B6E80").toUpperCase(),
      primary: bool("ui.primary_accent", false),
      allowed: {
        color_scheme: bool("ui.own_color_scheme", true),
        accent: bool("ui.own_accent", true),
        density: bool("ui.own_density", true),
        nav: bool("ui.own_nav", true),
      },
      scheme: text("ui.color_scheme", "system"),
      density: text("ui.density", "normal"),
      nav: text("ui.nav", "wide"),
    });
  }, [settings.data, form]);

  // The preview of a new logo, released when it goes.
  useEffect(() => {
    const urls: Partial<Record<Variant, string>> = {};
    for (const v of variants) {
      const f = logos[v];
      if (f instanceof File) {
        urls[v] = URL.createObjectURL(f);
      }
    }
    setPreviews(urls);
    return () => Object.values(urls).forEach((u) => URL.revokeObjectURL(u));
  }, [logos]);

  const save = useMutation({
    mutationFn: async (f: Form) => {
      await api.applyOrgSettings(node, {
        "ui.app_name": f.name.trim(),
        "ui.accent": f.accent,
        "ui.primary_accent": f.primary,
        "ui.own_color_scheme": f.allowed.color_scheme,
        "ui.own_accent": f.allowed.accent,
        "ui.own_density": f.allowed.density,
        "ui.own_nav": f.allowed.nav,
        "ui.color_scheme": f.scheme,
        "ui.density": f.density,
        "ui.nav": f.nav,
      });
      for (const v of variants) {
        const l = logos[v];
        if (l === "remove") {
          await api.removeOrgLogo(node, v);
        } else if (l instanceof File) {
          await api.setOrgLogo(node, v, l);
        }
      }
    },
    onMutate: () => {
      setMessage(null);
      setFieldErrors({});
    },
    onSuccess: () => {
      setLogos({});
      setMessage(t("appearance.saved"));
      void queryClient.invalidateQueries({ queryKey: ["me"] });
      void queryClient.invalidateQueries({ queryKey: ["org", "settings"] });
      void queryClient.invalidateQueries({ queryKey: ["branding"] });
    },
    onError: (err) => {
      if (err instanceof ApiError && err.status === 422) {
        setFieldErrors(Object.fromEntries(err.fieldErrors.map((e) => [e.field, e.message])));
        setMessage(t("appearance.invalid"));
        return;
      }
      setMessage(t("settings.error", { detail: err instanceof Error ? err.message : String(err) }));
    },
  });

  if (me.data && me.data.appearance !== true) {
    return <Alert variant="destructive">{t("appearance.forbidden")}</Alert>;
  }
  if (!form) {
    return settings.isError ? (
      <Alert variant="destructive">{t("settings.loadError")}</Alert>
    ) : (
      <p>{t("list.loading")}</p>
    );
  }

  const set = (change: Partial<Form>) => {
    setForm({ ...form, ...change });
    setFieldErrors({});
  };
  const validHex = hexRe.test(form.accent);
  const ratio = validHex ? contrast(form.accent, "#FFFFFF") : 0;
  const strong = validHex && ratio >= 4.5;
  const colorError = !validHex
    ? t("appearance.colorFormat")
    : !strong
      ? t("appearance.colorWeak", { ratio: ratio.toFixed(1).replace(".", ",") })
      : (fieldErrors["ui.accent"] ?? null);
  const nameError =
    form.name.trim() === "" ? t("appearance.nameRequired") : fieldErrors["ui.app_name"];
  const tokens = accentTokens(validHex ? form.accent : "#0B6E80");
  // The preview of the colour in the mode the page has: the variables --pick-… and the mode on
  // the element let the tokens give light or dark (UI55; before, the soft accent of light
  // stood white under light text in dark).
  const preview = {
    "--pick-accent-light": tokens.light,
    "--pick-accent-dark": tokens.dark,
    "--pick-accent-soft-light": tokens.softLight,
    "--pick-accent-soft-dark": tokens.softDark,
    ...(form.primary
      ? { "--pick-primary-light": tokens.light, "--pick-primary-dark": tokens.dark }
      : {}),
  } as CSSProperties;
  const pageMode = document.documentElement.dataset.colorScheme === "dark" ? "dark" : "light";
  const tenantName =
    labelOf(me.data?.tenants?.find((x) => x.path === node)?.label, i18n.language, "") ||
    (node === "root" ? t("appearance.allTenants") : node);
  const logoOf = (v: Variant) =>
    logos[v] === "remove"
      ? null
      : (previews[v] ??
        (v === "light" ? branding.data?.logo_light : branding.data?.logo_dark) ??
        null);
  const ownLabels: Record<Own, [string, string]> = {
    color_scheme: [t("settings.mode"), t("appearance.ownHint.color_scheme")],
    accent: [t("settings.accent"), t("appearance.ownHint.accent")],
    density: [t("settings.density"), t("appearance.ownHint.density")],
    nav: [t("settings.nav"), t("appearance.ownHint.nav")],
  };

  return (
    <div className="flex max-w-3xl flex-col gap-4">
      <div>
        <p className="text-sm text-muted-foreground">{t("nav.admin")}</p>
        <h1 className="text-xl font-semibold">{t("appearance.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("appearance.intro", { tenant: tenantName })}
        </p>
      </div>

      <Section title={t("appearance.brand")}>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-name`}>{t("appearance.name")}</Label>
          <Input
            id={`${id}-name`}
            value={form.name}
            maxLength={40}
            aria-invalid={nameError ? true : undefined}
            aria-describedby={`${id}-name-hint`}
            onChange={(e) => set({ name: e.target.value })}
          />
          <p
            id={`${id}-name-hint`}
            className={`text-sm ${nameError ? "text-destructive" : "text-muted-foreground"}`}
          >
            {nameError ?? t("appearance.nameHint")}
          </p>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          {variants.map((v) => {
            const src = logoOf(v);
            const error = fieldErrors[`logo_${v}`];
            return (
              <div key={v} className="flex flex-col gap-1.5">
                <Label htmlFor={`${id}-logo-${v}`}>{t(`appearance.logo.${v}`)}</Label>
                <div
                  className={`flex h-14 items-center justify-center rounded-md border ${v === "dark" ? "bg-[#111315]" : "bg-white"}`}
                >
                  {src ? (
                    <img
                      src={src}
                      alt={t(`appearance.logo.${v}`)}
                      className="max-h-10 max-w-40 object-contain"
                    />
                  ) : (
                    <span
                      className={`text-sm ${v === "dark" ? "text-[#b4b8be]" : "text-[#4a4f57]"}`}
                    >
                      {t("appearance.noLogo")}
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <input
                    id={`${id}-logo-${v}`}
                    type="file"
                    accept="image/png,image/svg+xml"
                    aria-describedby={`${id}-logo-hint`}
                    className="min-w-0 flex-1 text-sm file:mr-2 file:rounded-md file:border file:border-input file:bg-card file:px-2 file:py-1 file:text-sm"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        setLogos({ ...logos, [v]: file });
                      }
                    }}
                  />
                  {src && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setLogos({ ...logos, [v]: "remove" })}
                    >
                      {t("appearance.removeLogo")}
                    </Button>
                  )}
                </div>
                {error && <p className="text-sm text-destructive">{error}</p>}
              </div>
            );
          })}
        </div>
        <p id={`${id}-logo-hint`} className="text-sm text-muted-foreground">
          {t("appearance.logoHint")}
        </p>
      </Section>

      <Section title={t("appearance.color")} hint={t("appearance.colorHint")}>
        <div className="flex flex-wrap items-end gap-3">
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-color`}>{t("settings.accent")}</Label>
            <div className="flex items-center gap-2">
              <input
                type="color"
                aria-label={t("appearance.colorPicker")}
                value={validHex ? form.accent.toLowerCase() : "#0b6e80"}
                onChange={(e) => set({ accent: e.target.value.toUpperCase() })}
                className="h-10 w-12 cursor-pointer rounded-md border border-input bg-background p-1"
              />
              <Input
                id={`${id}-color`}
                value={form.accent}
                maxLength={7}
                className="w-28 font-mono"
                aria-invalid={colorError ? true : undefined}
                aria-describedby={`${id}-color-state`}
                onChange={(e) => set({ accent: e.target.value.trim() })}
              />
            </div>
          </div>
          <p
            id={`${id}-color-state`}
            className={`pb-2 text-sm ${colorError ? "text-destructive" : "text-muted-foreground"}`}
          >
            {colorError ?? t("appearance.colorOk", { ratio: ratio.toFixed(1).replace(".", ",") })}
          </p>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-4"
            checked={form.primary}
            onChange={(e) => set({ primary: e.target.checked })}
          />
          {t("appearance.primary")}
        </label>
        <div
          style={preview}
          data-color-scheme={pageMode}
          className="flex flex-wrap items-center gap-3 rounded-md border bg-background p-3"
          aria-label={t("appearance.preview")}
          role="group"
        >
          <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {t("appearance.preview")}
          </span>
          <span className="inline-flex h-9 items-center rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground">
            {t("form.save")}
          </span>
          <span className="text-sm text-accent underline">{t("appearance.previewLink")}</span>
          <span className="rounded-sm bg-accent-soft px-2 py-1 text-sm">
            {t("appearance.previewRow")}
          </span>
          <span className="rounded-sm px-2 py-1 text-sm ring-2 ring-ring">
            {t("appearance.previewFocus")}
          </span>
        </div>
      </Section>

      <Section title={t("appearance.own")} hint={t("appearance.ownIntro")}>
        <div className="flex flex-col gap-2">
          {own.map((k) => (
            <div key={k} className="flex items-start gap-2.5 text-sm">
              <input
                id={`${id}-own-${k}`}
                type="checkbox"
                className="mt-0.5 size-4"
                aria-describedby={`${id}-own-${k}-hint`}
                checked={form.allowed[k]}
                onChange={(e) => set({ allowed: { ...form.allowed, [k]: e.target.checked } })}
              />
              <span className="flex flex-col">
                <label htmlFor={`${id}-own-${k}`} className="font-medium">
                  {ownLabels[k][0]}
                </label>
                <span id={`${id}-own-${k}-hint`} className="text-muted-foreground">
                  {ownLabels[k][1]}
                </span>
              </span>
            </div>
          ))}
        </div>
      </Section>

      <Section title={t("appearance.defaults")} hint={t("appearance.defaultsHint")}>
        <div className="grid items-start gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,20rem)]">
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1">
              <Label htmlFor={`${id}-scheme`}>{t("settings.mode")}</Label>
              <select
                id={`${id}-scheme`}
                className={selectClass}
                value={form.scheme}
                onChange={(e) => set({ scheme: e.target.value })}
              >
                {(["system", "light", "dark"] as const).map((v) => (
                  <option key={v} value={v}>
                    {t(`colorScheme.${v}`)}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor={`${id}-density`}>{t("settings.density")}</Label>
              <select
                id={`${id}-density`}
                className={selectClass}
                value={form.density}
                onChange={(e) => set({ density: e.target.value })}
              >
                {(["normal", "compact"] as const).map((v) => (
                  <option key={v} value={v}>
                    {t(`list.densities.${v}`)}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor={`${id}-nav`}>{t("settings.navWidth")}</Label>
              <select
                id={`${id}-nav`}
                className={selectClass}
                value={form.nav}
                onChange={(e) => set({ nav: e.target.value })}
              >
                {(["wide", "narrow"] as const).map((v) => (
                  <option key={v} value={v}>
                    {t(`settings.navWidths.${v}`)}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <LookPreview
            scheme={form.scheme}
            density={form.density}
            nav={form.nav}
            accent={validHex ? form.accent : null}
            primary={form.primary}
            name={form.name.trim() || defaultName}
            logoLight={logoOf("light")}
            logoDark={logoOf("dark")}
          />
        </div>
      </Section>

      <div className="flex items-center gap-3">
        <Button
          type="button"
          disabled={save.isPending || colorError !== null || !!nameError}
          onClick={() => save.mutate(form)}
        >
          {t("form.save")}
        </Button>
        <span role="status" className="text-sm text-muted-foreground">
          {message}
        </span>
      </div>
    </div>
  );
}
