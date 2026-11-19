// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Mass actions on the chosen records of a list (UI6, Hauptplan E126): set a field, run a
// transition, set a role or move to the trash, in one call to :batch with the version each
// row had when the list loaded it. Rows changed meanwhile are conflicts and stay as they are.
// The choice stays after the run: the list loads again, so the chosen rows carry their new
// versions.
//
// UI61 (UI-Plan E59, E60): the action is one row in place of the column heads, so the table does
// not move: "4 ausgewählt", action, field and value, "Anwenden" without a number and "Auswahl
// aufheben". The result comes as a message: all changed green, some yellow with the refused
// rows and their reason, none red. The trash asks first (UI65); its message offers "Rückgängig".
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
import { Button } from "@/ui/button";
import { confirm } from "@/ui/confirm";
import { Input } from "@/ui/input";
import { Select, type ControlSize } from "@/ui/select";
import { toast } from "@/ui/toast";

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
  size = "form",
}: {
  id: string;
  field: Field;
  value: unknown;
  onChange: (v: unknown) => void;
  size?: ControlSize;
}) {
  const { t, i18n } = useTranslation();
  const label = t("batch.value");
  const select = (options: { value: string; label: string }[]) => (
    <Select
      id={id}
      label={label}
      size={size}
      className={size === "bar" ? "min-w-32" : undefined}
      value={String(value ?? "")}
      onChange={onChange}
      options={[{ value: "", label: t("batch.empty") }, ...options]}
    />
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
            size={size}
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
          size={size}
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
          size={size}
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
          size={size}
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
          size={size}
          aria-label={label}
          maxLength={field.max_length || undefined}
          value={String(value ?? "")}
          onChange={(e) => onChange(e.target.value)}
        />
      );
  }
}

export function BatchRow({
  type,
  rows,
  onClear,
}: {
  type: TypeDefinition;
  rows: RecordData[];
  onClear: () => void;
}) {
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
  const [busy, setBusy] = useState(false);
  const chosenField = fields.find((f) => f.name === field);

  const change = (): Record<string, unknown> => {
    switch (action) {
      case "field":
        return { set: { [field]: valueFor(chosenField!, value) } };
      case "transition":
        return { transition };
      case "role":
        return { set: { actors: { [role]: holders } } };
      case "trash":
        return { trash: true };
    }
  };

  const reload = () => void queryClient.invalidateQueries({ queryKey: ["records", typeName] });

  // undo takes the rows just moved to the trash out of it again.
  const undo = async (ids: string[]) => {
    try {
      await Promise.all(ids.map((rid) => api.restore(typeName, rid)));
      toast.success(t("batch.restored", { count: ids.length }));
    } catch (err) {
      toast.error(
        t("batch.restoreFailed", { detail: err instanceof Error ? err.message : String(err) }),
      );
    } finally {
      reload();
    }
  };

  const report = (results: Result[]) => {
    const titles = new Map(rows.map((r) => [String(r["id"]), titleOf(type, r)]));
    const done = results.filter((r) => r.status === "ok").map((r) => r.id);
    const refused = results.filter((r) => r.status !== "ok");
    const undoable =
      action === "trash" && done.length > 0
        ? { action: { label: t("batch.undo"), onClick: () => void undo(done) } }
        : undefined;
    const detail =
      refused.length > 0 ? (
        <ul className="list-disc pl-4">
          {refused.map((r) => (
            <li key={r.id}>
              {titles.get(r.id) ?? r.id}: {t(`batch.reason.${r.status}`)}
              {r.message && ` – ${r.message}`}
            </li>
          ))}
        </ul>
      ) : undefined;
    if (refused.length === 0) {
      toast.success(
        t(action === "trash" ? "batch.trashed" : "batch.done", { count: done.length }),
        undefined,
        undoable,
      );
    } else if (done.length > 0) {
      toast.warning(
        t(action === "trash" ? "batch.partlyTrashed" : "batch.partly", {
          ok: done.length,
          count: results.length,
        }),
        detail,
        undoable,
      );
    } else {
      toast.error(t("batch.none"), detail);
    }
  };

  const run = async () => {
    // Into the trash only after the common question with the number (UI65, E62).
    if (
      action === "trash" &&
      !(await confirm({
        title: t("batch.trashAsk", { count: rows.length }),
        text: t("trash.askText"),
        action: t("batch.actions.trash"),
      }))
    ) {
      return;
    }
    setBusy(true);
    try {
      const out = await api.batch(typeName, {
        rows: rows.map((r) => ({
          id: String(r["id"]),
          version: Number(r["version"]),
          ...change(),
        })),
      });
      report(out.results);
      reload();
    } catch (err) {
      toast.error(t("batch.error", { detail: err instanceof Error ? err.message : String(err) }));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      role="group"
      aria-labelledby={`${id}-count`}
      className="flex min-w-max items-center gap-2 font-normal"
    >
      <span id={`${id}-count`} className="font-medium whitespace-nowrap">
        {t("batch.title", { count: rows.length })}
      </span>
      <Select
        size="bar"
        label={t("batch.action")}
        value={action}
        onChange={(a) => setAction(a as Action)}
        options={actions.map((a) => ({ value: a, label: t(`batch.actions.${a}`) }))}
      />
      {action === "field" && (
        <>
          <Select
            size="bar"
            label={t("batch.field")}
            value={field}
            onChange={(f) => {
              setField(f);
              setValue("");
            }}
            options={fields.map((f) => ({ value: f.name, label: fieldLabel(f, i18n.language) }))}
          />
          {chosenField && (
            <FieldValue
              key={field}
              id={`${id}-value`}
              size="bar"
              field={chosenField}
              value={value}
              onChange={setValue}
            />
          )}
        </>
      )}
      {action === "transition" && (
        <Select
          size="bar"
          label={t("batch.transition")}
          value={transition}
          onChange={setTransition}
          options={transitions.map((tr) => ({
            value: tr.name,
            label: labelOf(tr.label, i18n.language, tr.name),
          }))}
        />
      )}
      {action === "role" && (
        <>
          <Select
            size="bar"
            label={t("batch.role")}
            value={role}
            onChange={(r) => {
              setRole(r);
              setHolders([]);
            }}
            options={roles.map((r) => ({
              value: r.name,
              label: labelOf(r.label, i18n.language, r.name),
            }))}
          />
          <div className="min-w-60">
            <RefPicker
              key={role}
              id={`${id}-holders`}
              size="bar"
              targets={roles.find((r) => r.name === role)?.to ?? []}
              single={roles.find((r) => r.name === role)?.single}
              value={holders}
              onChange={setHolders}
            />
          </div>
        </>
      )}
      <Button
        size="bar"
        variant={action === "trash" ? "destructive" : "default"}
        disabled={busy}
        onClick={() => void run()}
      >
        {t("batch.apply")}
      </Button>
      <span className="flex-1" />
      <Button size="bar" variant="ghost" onClick={onClear}>
        {t("batch.clear")}
      </Button>
    </div>
  );
}
