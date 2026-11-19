// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// "Globale Einstellungen" (UI9, SD20; Hauptplan E131, E180): the settings the packages declare,
// at a node of the organisation tree, grouped as the declarations say, each with a field of
// its kind and where its value comes from. Everything is changed first and saved at once in the
// bar at the top (your decision on the mockup "Einstellungen am Organisationsbaum").
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  labelOf,
  mySettingsQuery,
  type OrgNode,
  type SettingDefinition,
} from "@/api/client";
import { timeZones } from "@/app/SettingsPage";
import { RefPicker } from "@/render/RefPicker";
import { Button } from "@/ui/button";
import { Label } from "@/ui/label";
import { Select } from "@/ui/select";
import { Input } from "@/ui/input";

const mib = 1024 * 1024;

type Current = Record<string, { value: unknown; source: string }>;

// A change: a value, or null to remove the node's own one.
type Draft = Record<string, unknown>;

function nodeName(nodes: OrgNode[] | undefined, path: string, language: string): string {
  const n = nodes?.find((x) => x.path === path);
  return n ? labelOf(n.label, language, n.path) : path;
}

export function GlobalSettingsPage() {
  const { t, i18n } = useTranslation();
  const id = useId();
  const queryClient = useQueryClient();
  const [path, setPath] = useState("root");
  const [draft, setDraft] = useState<Draft>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [message, setMessage] = useState<string | null>(null);
  const nodes = useQuery({ queryKey: ["org", "nodes"], queryFn: api.orgNodes });
  const defs = useQuery({ queryKey: ["settings", "definitions"], queryFn: api.settingDefinitions });
  const current = useQuery({
    queryKey: ["org", "settings", path],
    queryFn: () => api.orgSettings(path),
  });
  const lang = i18n.language;
  const here = nodeName(nodes.data, path, lang);
  const values: Current = {};
  for (const s of current.data ?? []) {
    values[s.key] = { value: s.value, source: s.source };
  }
  const dirty = Object.keys(draft).length;

  const change = (key: string, value: unknown) => {
    setDraft((d) => ({ ...d, [key]: value }));
    setErrors((e) => Object.fromEntries(Object.entries(e).filter(([k]) => k !== key)));
    setMessage(null);
  };
  const discard = () => {
    setDraft({});
    setErrors({});
    setMessage(dirty ? t("settings.discarded") : null);
  };
  const save = () => {
    if (!dirty) return;
    void api.applyOrgSettings(path, draft).then(
      () => {
        setMessage(t("settings.saved", { count: dirty, node: here }));
        setDraft({});
        setErrors({});
        void queryClient.invalidateQueries({ queryKey: ["org", "settings"] });
        void queryClient.invalidateQueries({ queryKey: mySettingsQuery.queryKey });
      },
      (err: unknown) => {
        if (err instanceof ApiError && err.status === 403) {
          setMessage(t("settings.forbidden"));
          return;
        }
        if (
          err instanceof ApiError &&
          err.status === 422 &&
          err.problem &&
          "errors" in err.problem
        ) {
          const byKey: Record<string, string> = {};
          for (const e of err.problem.errors) byKey[e.field] = e.message;
          setErrors(byKey);
          setMessage(t("settings.invalid"));
          return;
        }
        setMessage(
          t("settings.error", { detail: err instanceof Error ? err.message : String(err) }),
        );
      },
    );
  };
  const pickNode = (next: string) => {
    if (dirty && !window.confirm(t("settings.discardAsk"))) return;
    setPath(next);
    setDraft({});
    setErrors({});
    setMessage(null);
  };

  // Sections in the order of the declarations: by group, else by package.
  const sections: { key: string; title: string; origin: string; defs: SettingDefinition[] }[] = [];
  for (const d of defs.data ?? []) {
    const title = d.group
      ? labelOf(d.group, lang, d.package)
      : t("settings.package", { name: d.package });
    const key = `${d.package}\u0000${title}`;
    let s = sections.find((x) => x.key === key);
    if (!s) {
      s = {
        key,
        title,
        origin:
          d.package === "core" ? t("settings.core") : t("settings.package", { name: d.package }),
        defs: [],
      };
      sections.push(s);
    }
    s.defs.push(d);
  }

  return (
    <div className="flex max-w-4xl flex-col gap-4">
      <div
        role="region"
        aria-label={t("settings.bar")}
        className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-background p-3"
      >
        <h1 className="text-xl font-semibold">{t("settings.globalOf", { node: here })}</h1>
        <div className="flex flex-wrap items-center gap-2">
          <span
            role="status"
            className={
              dirty
                ? "text-sm font-medium text-warning-foreground"
                : "text-sm text-muted-foreground"
            }
          >
            {dirty ? t("settings.unsaved", { count: dirty }) : t("settings.allSaved")}
          </span>
          <Button variant="outline" className="h-10" disabled={!dirty} onClick={discard}>
            {t("settings.discard")}
          </Button>
          <Button className="h-10" disabled={!dirty} onClick={save}>
            {t("form.save")}
          </Button>
        </div>
      </div>
      <p className="text-sm text-muted-foreground">{t("settings.globalHint")}</p>
      <div className="flex max-w-md flex-col gap-1">
        <Label htmlFor={`${id}-node`}>{t("settings.node")}</Label>
        <Select
          id={`${id}-node`}
          value={path}
          onChange={pickNode}
          options={(nodes.data ?? [{ path: "root", name: "root", label: {} }]).map((n) => ({
            value: n.path,
            label: `${labelOf(n.label, lang, n.path)} (${n.path})`,
          }))}
        />
      </div>
      {message && (
        <p className="rounded-md border bg-muted p-3 text-sm" role="alert">
          {message}
        </p>
      )}
      {(defs.isError || current.isError) && <p role="alert">{t("settings.loadError")}</p>}
      {sections.map((s) => (
        <section
          key={s.key}
          aria-labelledby={`${id}-${s.key}`}
          className="flex flex-col gap-3 rounded-lg border p-4"
        >
          <h2 id={`${id}-${s.key}`} className="text-base font-semibold">
            {s.title}{" "}
            <span className="ml-1 text-sm font-normal text-muted-foreground">{s.origin}</span>
          </h2>
          {s.defs.map((d) => (
            <Setting
              key={d.key}
              def={d}
              path={path}
              current={values[d.key]}
              draft={draft}
              error={errors[d.key]}
              nodeLabel={(p) => nodeName(nodes.data, p, lang)}
              onChange={(v) => change(d.key, v)}
            />
          ))}
        </section>
      ))}
    </div>
  );
}

type SettingProps = {
  def: SettingDefinition;
  path: string;
  current: { value: unknown; source: string } | undefined;
  draft: Draft;
  error: string | undefined;
  nodeLabel: (path: string) => string;
  onChange: (value: unknown) => void;
};

// Setting is one setting with its field, where it comes from and "Eigene entfernen".
function Setting({ def, path, current, draft, error, nodeLabel, onChange }: SettingProps) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const lang = i18n.language;
  const own = current?.source === path;
  const changed = def.key in draft;
  const removing = changed && draft[def.key] === null;
  const inherited = current && !own ? current : undefined;
  const value =
    changed && !removing
      ? draft[def.key]
      : removing
        ? (inherited?.value ?? def.default)
        : (current?.value ?? def.default);
  const locked = !!def.root && path !== "root";
  const label = labelOf(def.label, lang, def.key);
  const hintId = `${id}-hint`;
  const errId = `${id}-err`;
  const described =
    [def.description ? hintId : "", error ? errId : ""].filter(Boolean).join(" ") || undefined;

  let source: string;
  if (removing) source = t("settings.removing");
  else if (changed) source = t("settings.changed");
  else if (own) source = t("settings.own");
  else if (inherited) source = t("settings.inherited", { node: nodeLabel(inherited.source) });
  else if (def.default !== undefined)
    source =
      def.package === "core"
        ? t("settings.defaultCore")
        : t("settings.defaultPackage", { name: def.package });
  else source = t("settings.none");

  return (
    <div
      role="group"
      aria-labelledby={`${id}-label`}
      className="flex flex-col gap-2 rounded-md border p-3"
    >
      {def.kind === "boolean" ? (
        <span id={`${id}-label`} className="sr-only">
          {label}
        </span>
      ) : (
        <Label
          id={`${id}-label`}
          htmlFor={def.kind === "matrix" || def.kind === "list" ? undefined : `${id}-field`}
        >
          {label}
        </Label>
      )}
      {def.description && (
        <p id={hintId} className="text-sm text-muted-foreground">
          {labelOf(def.description, lang, "")}
        </p>
      )}
      <Field
        def={def}
        id={`${id}-field`}
        value={value}
        disabled={locked}
        describedBy={described}
        invalid={!!error}
        label={label}
        onChange={onChange}
      />
      {error && (
        <p id={errId} className="text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        <span className={own && !changed ? "text-sm text-accent" : "text-sm text-muted-foreground"}>
          {source}
        </span>
        {own && !removing && !locked && (
          <Button size="sm" variant="outline" onClick={() => onChange(null)}>
            {t("settings.removeOwn")}
          </Button>
        )}
      </div>
      {locked && <p className="text-sm text-muted-foreground">{t("settings.rootOnly")}</p>}
    </div>
  );
}

type FieldProps = {
  def: SettingDefinition;
  id: string;
  value: unknown;
  disabled: boolean;
  invalid: boolean;
  describedBy: string | undefined;
  label: string;
  onChange: (value: unknown) => void;
};

function numberText(v: unknown, divide = 1): string {
  return typeof v === "number" ? String(Math.round((v / divide) * 100) / 100) : "";
}

// Field is the input of a setting by its kind.
function Field({ def, id, value, disabled, invalid, describedBy, label, onChange }: FieldProps) {
  const { t, i18n } = useTranslation();
  const lang = i18n.language;
  const unit = def.unit ? labelOf(def.unit, lang, "") : "";
  const common = {
    id,
    disabled,
    "aria-invalid": invalid || undefined,
    "aria-describedby": describedBy,
  };
  const numeric = (scale: number, suffix: string) => (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        {...common}
        type="number"
        inputMode="decimal"
        step="any"
        className="w-32"
        value={numberText(value, scale)}
        onChange={(e) => {
          const n = Number(e.target.value);
          // Durations and sizes are whole minutes and bytes.
          onChange(e.target.value === "" ? "" : scale === 1 ? n : Math.round(n * scale));
        }}
      />
      {suffix && <span className="text-sm text-muted-foreground">{suffix}</span>}
    </div>
  );
  switch (def.kind) {
    case "choice":
      return (
        <Select
          id={id}
          disabled={disabled}
          invalid={invalid}
          describedBy={describedBy}
          wrapClassName="max-w-md"
          value={typeof value === "string" ? value : ""}
          onChange={onChange}
          options={(def.options ?? []).map((o) => ({
            value: String(o.value),
            label: labelOf(o.label, lang, String(o.value)),
          }))}
        />
      );
    case "time_zone":
      return (
        <Select
          id={id}
          disabled={disabled}
          invalid={invalid}
          describedBy={describedBy}
          wrapClassName="max-w-md"
          value={typeof value === "string" ? value : ""}
          onChange={onChange}
          options={timeZones().map((z) => ({ value: z, label: z }))}
        />
      );
    case "text":
      return (
        <Input
          {...common}
          className="max-w-md"
          value={typeof value === "string" ? value : ""}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    case "boolean":
      return (
        <label className="flex items-center gap-2 text-sm">
          <input
            {...common}
            type="checkbox"
            checked={value === true}
            onChange={(e) => onChange(e.target.checked)}
          />
          {label}
        </label>
      );
    case "number":
      return numeric(1, unit);
    case "duration":
      // Stored in minutes, shown in hours.
      return numeric(60, t("settings.hours"));
    case "size":
      return numeric(mib, t("settings.mb"));
    case "list":
      return (
        <ListField
          id={id}
          value={value}
          disabled={disabled}
          describedBy={describedBy}
          label={label}
          onChange={onChange}
        />
      );
    case "matrix":
      return <MatrixField def={def} value={value} disabled={disabled} onChange={onChange} />;
    case "reference":
      return (
        <div className="max-w-md">
          <RefPicker
            id={id}
            targets={def.to ? [def.to] : []}
            single
            disabled={disabled}
            invalid={invalid}
            describedBy={describedBy}
            value={typeof value === "string" && def.to ? [{ type: def.to, id: value }] : []}
            onChange={(refs) => onChange(refs[0]?.id ?? null)}
          />
        </div>
      );
  }
  return null;
}

// ListField is a list of texts as marks to remove and a field to add one.
function ListField({
  id,
  value,
  disabled,
  describedBy,
  label,
  onChange,
}: {
  id: string;
  value: unknown;
  disabled: boolean;
  describedBy: string | undefined;
  label: string;
  onChange: (value: unknown) => void;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const list = Array.isArray(value) ? (value as unknown[]).map(String) : [];
  const add = () => {
    const entry = text.trim();
    if (!entry || list.includes(entry)) return;
    onChange([...list, entry]);
    setText("");
  };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      add();
    }
  };
  return (
    <div className="flex flex-wrap items-center gap-2">
      {list.length === 0 && (
        <span className="text-sm text-muted-foreground">{t("settings.empty")}</span>
      )}
      {list.map((entry) => (
        <span
          key={entry}
          className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-sm"
        >
          {entry}
          <button
            type="button"
            className="rounded px-1 text-muted-foreground hover:text-foreground"
            disabled={disabled}
            aria-label={t("settings.removeEntry", { entry })}
            onClick={() => onChange(list.filter((x) => x !== entry))}
          >
            ×
          </button>
        </span>
      ))}
      <Input
        id={id}
        className="w-44"
        disabled={disabled}
        aria-label={t("settings.addEntryTo", { name: label })}
        aria-describedby={describedBy}
        placeholder={t("settings.addEntry")}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={onKeyDown}
      />
      <Button size="sm" variant="outline" disabled={disabled || !text.trim()} onClick={add}>
        {t("settings.add")}
      </Button>
    </div>
  );
}

// MatrixField is a table of rows and columns with a choice in each cell.
function MatrixField({
  def,
  value,
  disabled,
  onChange,
}: {
  def: SettingDefinition;
  value: unknown;
  disabled: boolean;
  onChange: (value: unknown) => void;
}) {
  const { t, i18n } = useTranslation();
  const lang = i18n.language;
  const rows = def.rows?.options ?? [];
  const columns = def.columns?.options ?? [];
  const m = (value && typeof value === "object" ? value : {}) as Record<
    string,
    Record<string, unknown>
  >;
  const fallback = (def.default ?? {}) as Record<string, Record<string, unknown>>;
  const options = def.options ?? [];
  const rowName = labelOf(def.rows?.label, lang, "");
  const colName = labelOf(def.columns?.label, lang, "");
  const set = (r: string, c: string, raw: string) => {
    const pick = options.find((o) => String(o.value) === raw)?.value ?? raw;
    const next: Record<string, Record<string, unknown>> = {};
    for (const row of rows) {
      const rk = String(row.value);
      next[rk] = { ...(fallback[rk] ?? {}), ...(m[rk] ?? {}) };
    }
    next[r] = { ...next[r], [c]: pick };
    onChange(next);
  };
  return (
    <div className="overflow-x-auto">
      <table className="border-collapse text-sm">
        <thead>
          <tr>
            <th scope="col" className="border px-2 py-1 text-left font-medium">
              {t("settings.matrixCorner", { rows: rowName, columns: colName })}
            </th>
            {columns.map((c) => (
              <th key={String(c.value)} scope="col" className="border px-2 py-1 font-medium">
                {labelOf(c.label, lang, String(c.value))}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const rk = String(r.value);
            const rl = labelOf(r.label, lang, rk);
            return (
              <tr key={rk}>
                <th scope="row" className="border px-2 py-1 text-left font-medium">
                  {rl}
                </th>
                {columns.map((c) => {
                  const ck = String(c.value);
                  const cl = labelOf(c.label, lang, ck);
                  const cell = m[rk]?.[ck] ?? fallback[rk]?.[ck];
                  const differs = String(cell) !== String(fallback[rk]?.[ck]);
                  return (
                    <td key={ck} className="border px-2 py-1 text-center">
                      <Select
                        className={differs ? "border-accent ring-1 ring-accent" : undefined}
                        label={t("settings.matrixCell", {
                          row: rowName,
                          rowValue: rl,
                          column: colName,
                          columnValue: cl,
                        })}
                        disabled={disabled}
                        value={String(cell ?? "")}
                        onChange={(v) => set(rk, ck, v)}
                        options={options.map((o) => ({
                          value: String(o.value),
                          label: labelOf(o.label, lang, String(o.value)),
                        }))}
                      />
                    </td>
                  );
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
      <p className="mt-1 text-sm text-muted-foreground">{t("settings.matrixHint")}</p>
    </div>
  );
}
