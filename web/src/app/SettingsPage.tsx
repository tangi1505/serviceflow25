// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Settings (UI9, Hauptplan E131). "Meine Einstellungen": language, time zone and theme, each
// own or "as set for my unit", with where the value in force comes from; presets per type
// for creating. "Globale Einstellungen": the defaults at a node of the organisation tree,
// for those who hold manage_settings there (Kern-Plan E59).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  labelOf,
  meQuery,
  mySettingsQuery,
  qualifiedName,
  typesQuery,
  type MySettings,
} from "@/api/client";
import { languages } from "@/i18n";
import { FieldValue } from "@/render/Batch";
import { fieldLabel, isCalculated } from "@/render/fields";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Label } from "@/ui/label";
import { themes } from "@/ui/theme";

const selectClass = "h-10 rounded-md border border-input bg-background px-3 text-sm";

// timeZones are the zones the browser knows; Europe/Berlin first.
function timeZones(): string[] {
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

type Key = "language" | "time_zone" | "theme";

function Choice({
  name,
  settings,
  value,
  onChange,
  options,
}: {
  name: Key;
  settings: MySettings;
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
}) {
  const { t } = useTranslation();
  const id = useId();
  const source = useSource();
  const now = settings[name];
  return (
    <div className="flex flex-col gap-1">
      <Label htmlFor={id}>{t(`settings.${name}`)}</Label>
      <select
        id={id}
        className={selectClass}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        <option value="">{t("settings.inherit")}</option>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      <p className="text-sm text-muted-foreground">
        {t("settings.inForce", { value: now.value, source: source(now.source) })}
      </p>
    </div>
  );
}

function Presets({ settings }: { settings: MySettings }) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const queryClient = useQueryClient();
  const types = useQuery(typesQuery);
  const [typeName, setTypeName] = useState("");
  const [rows, setRows] = useState<{ field: string; value: unknown }[]>([]);
  const [message, setMessage] = useState<string | null>(null);
  const type = types.data?.find((x) => qualifiedName(x) === typeName);
  const fields = (type?.fields ?? []).filter((f) => !isCalculated(f) && f.format !== "rich");
  const pick = (name: string) => {
    setTypeName(name);
    setMessage(null);
    setRows(
      Object.entries(settings.defaults[name] ?? {}).map(([field, value]) => ({
        field,
        value:
          typeof value === "boolean" || typeof value === "number" ? value : String(value ?? ""),
      })),
    );
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
      setMessage(t("form.saved"));
    },
    onError: (err) =>
      setMessage(t("settings.error", { detail: err instanceof Error ? err.message : String(err) })),
  });
  return (
    <section aria-labelledby={`${id}-title`} className="flex flex-col gap-3 rounded-md border p-4">
      <h2 id={`${id}-title`} className="text-lg font-semibold">
        {t("settings.presets")}
      </h2>
      <p className="text-sm text-muted-foreground">{t("settings.presetsHint")}</p>
      <div className="flex flex-col gap-1">
        <Label htmlFor={`${id}-type`}>{t("settings.type")}</Label>
        <select
          id={`${id}-type`}
          className={selectClass}
          value={typeName}
          onChange={(e) => pick(e.target.value)}
        >
          <option value="">{t("form.none")}</option>
          {(types.data ?? []).map((x) => (
            <option key={qualifiedName(x)} value={qualifiedName(x)}>
              {labelOf(x.label, i18n.language, qualifiedName(x))}
            </option>
          ))}
        </select>
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
                  <select
                    aria-label={t("batch.field")}
                    className={selectClass}
                    value={r.field}
                    onChange={(e) =>
                      setRows(
                        rows.map((x, j) => (j === i ? { field: e.target.value, value: "" } : x)),
                      )
                    }
                  >
                    {fields.map((x) => (
                      <option key={x.name} value={x.name}>
                        {fieldLabel(x, i18n.language)}
                      </option>
                    ))}
                  </select>
                  {f && (
                    <FieldValue
                      key={r.field}
                      id={`${id}-v${i}`}
                      field={f}
                      value={r.value}
                      onChange={(v) =>
                        setRows(rows.map((x, j) => (j === i ? { ...x, value: v } : x)))
                      }
                    />
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-10"
                    onClick={() => setRows(rows.filter((_, j) => j !== i))}
                  >
                    {t("filter.remove")}
                  </Button>
                </li>
              );
            })}
          </ul>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={fields.length === 0}
              onClick={() => setRows([...rows, { field: fields[0]!.name, value: "" }])}
            >
              {t("settings.addPreset")}
            </Button>
            <Button size="sm" disabled={save.isPending} onClick={() => save.mutate()}>
              {t("settings.savePresets")}
            </Button>
          </div>
        </>
      )}
      {message && (
        <p role="status" className="text-sm">
          {message}
        </p>
      )}
    </section>
  );
}

export function MySettingsPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  // In the portal (UI10) presets and the global settings are of no use.
  const me = useQuery(meQuery);
  const settings = useQuery(mySettingsQuery);
  const own = (s: MySettings, k: Key) => (s[k].source === "own" ? s[k].value : "");
  const [draft, setDraft] = useState<Record<Key, string> | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: (d: Record<Key, string>) =>
      api.setMySettings({
        language: (d.language || null) as "de" | "en" | null,
        time_zone: d.time_zone || null,
        theme: d.theme || null,
      }),
    onSuccess: (saved) => {
      queryClient.setQueryData(mySettingsQuery.queryKey, saved);
      setDraft(null);
      setMessage(t("form.saved"));
    },
    onError: (err) =>
      setMessage(t("settings.error", { detail: err instanceof Error ? err.message : String(err) })),
  });
  if (!settings.data) {
    return settings.isError ? (
      <Alert variant="destructive">{t("settings.loadError")}</Alert>
    ) : (
      <p>{t("list.loading")}</p>
    );
  }
  const s = settings.data;
  const d = draft ?? {
    language: own(s, "language"),
    time_zone: own(s, "time_zone"),
    theme: own(s, "theme"),
  };
  const set = (k: Key) => (v: string) => setDraft({ ...d, [k]: v });
  return (
    <div className="flex max-w-2xl flex-col gap-6">
      <h1 className="text-2xl font-semibold">{t("settings.title")}</h1>
      <form
        className="flex flex-col gap-4 rounded-md border p-4"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate(d);
        }}
      >
        <Choice
          name="language"
          settings={s}
          value={d.language}
          onChange={set("language")}
          options={Object.keys(languages).map((l) => ({ value: l, label: t(`language.${l}`) }))}
        />
        <Choice
          name="time_zone"
          settings={s}
          value={d.time_zone}
          onChange={set("time_zone")}
          options={timeZones().map((z) => ({ value: z, label: z }))}
        />
        <Choice
          name="theme"
          settings={s}
          value={d.theme}
          onChange={set("theme")}
          options={themes.map((x) => ({ value: x, label: t(`theme.${x}`) }))}
        />
        <div className="flex items-center gap-3">
          <Button type="submit" disabled={save.isPending}>
            {t("form.save")}
          </Button>
          {message && (
            <span role="status" className="text-sm">
              {message}
            </span>
          )}
        </div>
      </form>
      {me.data?.workplace !== false && (
        <>
          <Presets settings={s} />
          <p className="text-sm">
            <Link to="/settings/global" className="underline">
              {t("settings.global")}
            </Link>
          </p>
        </>
      )}
    </div>
  );
}

const globalKeys = [
  { key: "ui.language", name: "language" },
  { key: "ui.time_zone", name: "time_zone" },
  { key: "ui.theme", name: "theme" },
] as const;

export function GlobalSettingsPage() {
  const { t, i18n } = useTranslation();
  const id = useId();
  const queryClient = useQueryClient();
  const [path, setPath] = useState("root");
  const nodes = useQuery({ queryKey: ["org", "nodes"], queryFn: api.orgNodes });
  const current = useQuery({
    queryKey: ["org", "settings", path],
    queryFn: () => api.orgSettings(path),
  });
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [message, setMessage] = useState<string | null>(null);
  const done = (msg: string) => {
    setMessage(msg);
    void queryClient.invalidateQueries({ queryKey: ["org", "settings"] });
    void queryClient.invalidateQueries({ queryKey: mySettingsQuery.queryKey });
  };
  const fail = (err: unknown) =>
    setMessage(
      err instanceof ApiError && err.status === 403
        ? t("settings.forbidden")
        : t("settings.error", { detail: err instanceof Error ? err.message : String(err) }),
    );
  const options = (name: string) =>
    name === "language"
      ? Object.keys(languages).map((l) => ({ value: l, label: t(`language.${l}`) }))
      : name === "theme"
        ? themes.map((x) => ({ value: x, label: t(`theme.${x}`) }))
        : timeZones().map((z) => ({ value: z, label: z }));
  return (
    <div className="flex max-w-2xl flex-col gap-6">
      <h1 className="text-2xl font-semibold">{t("settings.globalTitle")}</h1>
      <p className="text-sm text-muted-foreground">{t("settings.globalHint")}</p>
      <div className="flex flex-col gap-1">
        <Label htmlFor={`${id}-node`}>{t("settings.node")}</Label>
        <select
          id={`${id}-node`}
          className={selectClass}
          value={path}
          onChange={(e) => {
            setPath(e.target.value);
            setDraft({});
            setMessage(null);
          }}
        >
          {(nodes.data ?? [{ path: "root", name: "root", label: {} }]).map((n) => (
            <option key={n.path} value={n.path}>
              {labelOf(n.label, i18n.language, n.path)} ({n.path})
            </option>
          ))}
        </select>
      </div>
      <ul className="flex flex-col gap-4">
        {globalKeys.map(({ key, name }) => {
          const s = current.data?.find((x) => x.key === key);
          const value = draft[key] ?? (typeof s?.value === "string" ? s.value : "");
          return (
            <li
              key={key}
              className="flex flex-col gap-1 rounded-md border p-3"
              aria-label={t(`settings.${name}`)}
            >
              <Label htmlFor={`${id}-${name}`}>{t(`settings.${name}`)}</Label>
              <div className="flex flex-wrap gap-2">
                <select
                  id={`${id}-${name}`}
                  className={selectClass}
                  value={value}
                  onChange={(e) => setDraft({ ...draft, [key]: e.target.value })}
                >
                  <option value="">{t("form.none")}</option>
                  {options(name).map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
                <Button
                  size="sm"
                  className="h-10"
                  disabled={!value}
                  onClick={() =>
                    void api.setOrgSetting(path, key, value).then(() => done(t("form.saved")), fail)
                  }
                >
                  {t("form.save")}
                </Button>
                {s?.source === path && (
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-10"
                    onClick={() =>
                      void api
                        .unsetOrgSetting(path, key)
                        .then(() => done(t("settings.removed")), fail)
                    }
                  >
                    {t("settings.remove")}
                  </Button>
                )}
              </div>
              <p className="text-sm text-muted-foreground">
                {s ? t("settings.from", { node: s.source }) : t("settings.notSet")}
              </p>
            </li>
          );
        })}
      </ul>
      {message && (
        <p role="status" className="text-sm">
          {message}
        </p>
      )}
    </div>
  );
}
