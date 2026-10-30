// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Mass actions on the chosen records of a list (UI6, Hauptplan E126): set a field, run a
// transition, set a role or move to the trash, in one call to :batch with the version each
// row had when the list loaded it. The result says per row what happened; rows changed
// meanwhile are conflicts and stay as they are. The choice stays after the run: the list
// loads again, so the chosen rows carry their new versions.
import { useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  qualifiedName,
  type ActorRef,
  type Field,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";

import { fieldLabel, isCalculated, optionLabel } from "./fields";
import { RefPicker } from "./RefPicker";
import { titleOf } from "./RefName";

type Action = "field" | "transition" | "role" | "trash";
type Result = { id: string; status: string; message?: string };

// valueFor turns the input of a field into the value the API takes; "" clears.
function valueFor(f: Field, raw: unknown): unknown {
  if (raw === "" || raw === undefined) {
    return null;
  }
  switch (f.type) {
    case "number":
      return Number(raw);
    case "boolean":
      return raw === true || raw === "true";
    case "datetime":
      return new Date(String(raw)).toISOString();
    default:
      return raw;
  }
}

export function FieldValue({
  id,
  field,
  value,
  onChange,
}: {
  id: string;
  field: Field;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const { t, i18n } = useTranslation();
  const label = t("batch.value");
  const select = (options: { value: string; label: string }[]) => (
    <select
      id={id}
      aria-label={label}
      className="h-10 rounded-md border border-input bg-background px-3 text-sm"
      value={String(value ?? "")}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="">{t("batch.empty")}</option>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
  switch (field.type) {
    case "choice":
      return select(
        (field.options ?? []).map((o) => ({
          value: o.value,
          label: optionLabel(field, o.value, i18n.language),
        })),
      );
    case "boolean":
      return select([
        { value: "true", label: t("filter.yes") },
        { value: "false", label: t("filter.no") },
      ]);
    case "reference": {
      const v = String(value ?? "");
      return (
        <div className="min-w-60">
          <RefPicker
            id={id}
            targets={[field.to ?? ""]}
            single
            value={v ? [{ type: field.to ?? "", id: v }] : []}
            onChange={(refs) => onChange(refs[0]?.id ?? "")}
          />
        </div>
      );
    }
    case "number":
      return (
        <Input
          id={id}
          aria-label={label}
          type="number"
          step="any"
          value={String(value ?? "")}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    case "date":
      return (
        <Input
          id={id}
          aria-label={label}
          type="date"
          value={String(value ?? "")}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    case "datetime":
      return (
        <Input
          id={id}
          aria-label={label}
          type="datetime-local"
          value={String(value ?? "")}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    default:
      return (
        <Input
          id={id}
          aria-label={label}
          maxLength={field.max_length || undefined}
          value={String(value ?? "")}
          onChange={(e) => onChange(e.target.value)}
        />
      );
  }
}

export function BatchBar({ type, rows }: { type: TypeDefinition; rows: RecordData[] }) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const queryClient = useQueryClient();
  const typeName = qualifiedName(type);
  const fields = type.fields.filter((f) => !isCalculated(f));
  const roles = type.actors ?? [];
  const transitions = type.transitions ?? [];
  const actions: Action[] = [
    "field",
    ...(transitions.length ? (["transition"] as const) : []),
    ...(roles.length ? (["role"] as const) : []),
    "trash",
  ];
  const [action, setAction] = useState<Action>("field");
  const [field, setField] = useState(fields[0]?.name ?? "");
  const [value, setValue] = useState<unknown>("");
  const [transition, setTransition] = useState(transitions[0]?.name ?? "");
  const [role, setRole] = useState(roles[0]?.name ?? "");
  const [holders, setHolders] = useState<ActorRef[]>([]);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [results, setResults] = useState<Result[] | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const titles = new Map(rows.map((r) => [String(r["id"]), titleOf(type, r)]));

  const change = (): Record<string, unknown> => {
    switch (action) {
      case "field": {
        const f = fields.find((x) => x.name === field)!;
        return { set: { [field]: valueFor(f, value) } };
      }
      case "transition":
        return { transition };
      case "role":
        return { set: { actors: { [role]: holders } } };
      case "trash":
        return { trash: true };
    }
  };

  const run = async () => {
    setBusy(true);
    setFailure(null);
    setResults(null);
    try {
      const out = await api.batch(typeName, {
        rows: rows.map((r) => ({
          id: String(r["id"]),
          version: Number(r["version"]),
          ...change(),
        })),
      });
      setResults(out.results);
      void queryClient.invalidateQueries({ queryKey: ["records", typeName] });
    } catch (err) {
      setFailure(t("batch.error", { detail: err instanceof Error ? err.message : String(err) }));
    } finally {
      setBusy(false);
      setConfirm(false);
    }
  };

  const counts = new Map<string, number>();
  for (const r of results ?? []) {
    counts.set(r.status, (counts.get(r.status) ?? 0) + 1);
  }
  const selectClass = "h-10 rounded-md border border-input bg-background px-3 text-sm";
  return (
    <section
      aria-labelledby={`${id}-title`}
      className="flex flex-col gap-2 rounded-md border bg-card p-3"
    >
      <h2 id={`${id}-title`} className="text-sm font-semibold">
        {t("batch.title", { count: rows.length })}
      </h2>
      <div className="flex flex-wrap items-start gap-2">
        <select
          aria-label={t("batch.action")}
          className={selectClass}
          value={action}
          onChange={(e) => {
            setAction(e.target.value as Action);
            setConfirm(false);
          }}
        >
          {actions.map((a) => (
            <option key={a} value={a}>
              {t(`batch.actions.${a}`)}
            </option>
          ))}
        </select>
        {action === "field" && (
          <>
            <select
              aria-label={t("batch.field")}
              className={selectClass}
              value={field}
              onChange={(e) => {
                setField(e.target.value);
                setValue("");
              }}
            >
              {fields.map((f) => (
                <option key={f.name} value={f.name}>
                  {fieldLabel(f, i18n.language)}
                </option>
              ))}
            </select>
            {fields.find((f) => f.name === field) && (
              <FieldValue
                key={field}
                id={`${id}-value`}
                field={fields.find((f) => f.name === field)!}
                value={value}
                onChange={setValue}
              />
            )}
          </>
        )}
        {action === "transition" && (
          <select
            aria-label={t("batch.transition")}
            className={selectClass}
            value={transition}
            onChange={(e) => setTransition(e.target.value)}
          >
            {transitions.map((tr) => (
              <option key={tr.name} value={tr.name}>
                {labelOf(tr.label, i18n.language, tr.name)}
              </option>
            ))}
          </select>
        )}
        {action === "role" && (
          <>
            <select
              aria-label={t("batch.role")}
              className={selectClass}
              value={role}
              onChange={(e) => {
                setRole(e.target.value);
                setHolders([]);
              }}
            >
              {roles.map((r) => (
                <option key={r.name} value={r.name}>
                  {labelOf(r.label, i18n.language, r.name)}
                </option>
              ))}
            </select>
            <div className="min-w-60">
              <RefPicker
                key={role}
                id={`${id}-holders`}
                targets={roles.find((r) => r.name === role)?.to ?? []}
                single={roles.find((r) => r.name === role)?.single}
                value={holders}
                onChange={setHolders}
              />
            </div>
          </>
        )}
        {action === "trash" && confirm ? (
          <Button variant="destructive" disabled={busy} onClick={() => void run()}>
            {t("batch.confirmTrash", { count: rows.length })}
          </Button>
        ) : (
          <Button
            disabled={busy}
            onClick={() => (action === "trash" ? setConfirm(true) : void run())}
          >
            {t("batch.run", { count: rows.length })}
          </Button>
        )}
      </div>
      {failure && <Alert variant="destructive">{failure}</Alert>}
      {results && (
        <div role="status" className="text-sm">
          <p>
            {[...counts.entries()]
              .map(([status, n]) => t(`batch.status.${status}`, { count: n }))
              .join(" · ")}
          </p>
          {results.some((r) => r.status !== "ok") && (
            <ul className="mt-1 list-disc pl-5">
              {results
                .filter((r) => r.status !== "ok")
                .map((r) => (
                  <li key={r.id}>
                    {titles.get(r.id) ?? r.id}: {t(`batch.status.${r.status}`, { count: 1 })}
                    {r.message && <span className="text-muted-foreground"> – {r.message}</span>}
                  </li>
                ))}
            </ul>
          )}
        </div>
      )}
    </section>
  );
}
