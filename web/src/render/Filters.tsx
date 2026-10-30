// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The filters of a list (UI4, Hauptplan E124): conditions "field – comparison – value", all
// joined with AND, sent as the where of the JSON query (E40). The comparisons follow the kind
// of the field; references and roles are chosen with the picker (UI28), a role also as "me" or
// "my groups".
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { labelOf, type ActorRef, type Field, type State, type TypeDefinition } from "@/api/client";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";

import { fieldLabel, optionLabel } from "./fields";
import { RefPicker } from "./RefPicker";

export type Op = "eq" | "neq" | "contains" | "starts" | "gte" | "lte" | "null";
export type Condition = { field: string; op: Op; value: unknown };

// A filterable thing of the type: a field, the state, a system time or a role.
type Subject =
  | { kind: "field"; name: string; field: Field }
  | { kind: "state"; name: "state"; states: State[] }
  | { kind: "time"; name: "created_at" | "updated_at" }
  | { kind: "role"; name: string; to: string[]; label?: Record<string, string> };

function subjects(type: TypeDefinition): Subject[] {
  return [
    ...type.fields.map<Subject>((f) => ({ kind: "field", name: f.name, field: f })),
    ...(type.states?.length
      ? [{ kind: "state", name: "state", states: type.states } as const]
      : []),
    { kind: "time", name: "created_at" },
    { kind: "time", name: "updated_at" },
    ...(type.actors ?? []).map<Subject>((r) => ({
      kind: "role",
      name: `actors.${r.name}`,
      to: r.to,
      label: r.label,
    })),
  ];
}

// opsFor are the comparisons that make sense for a subject.
export function opsFor(s: Subject): Op[] {
  switch (s.kind) {
    case "state":
      return ["eq", "neq"];
    case "time":
      return ["gte", "lte"];
    case "role":
      return ["eq", "null"];
  }
  switch (s.field.type) {
    case "text":
      return ["contains", "eq", "starts", "null"];
    case "number":
    case "date":
      return ["eq", "gte", "lte", "null"];
    case "datetime":
      return ["gte", "lte", "null"];
    case "choice":
      return ["eq", "neq", "null"];
    case "boolean":
      return ["eq"];
    case "reference":
      return ["eq", "null"];
  }
}

// isMoment tells the subjects whose value is a moment: the form takes a day, the query gets
// its start (from) or its end (until) in the local time zone.
function isMoment(type: TypeDefinition, field: string): boolean {
  return (
    field === "created_at" ||
    field === "updated_at" ||
    type.fields.some((f) => f.name === field && f.type === "datetime")
  );
}

// toWhere turns the conditions into the where of the JSON query; incomplete ones are left out.
export function toWhere(
  type: TypeDefinition,
  conditions: Condition[],
): Record<string, unknown> | undefined {
  const parts = conditions
    .filter((c) => c.op === "null" || (c.value !== "" && c.value !== undefined && c.value !== null))
    .map((c) => {
      if (c.op === "null") {
        return { field: c.field, null: true };
      }
      let v = c.value;
      if (isMoment(type, c.field) && typeof v === "string" && /^\d{4}-\d{2}-\d{2}$/.test(v)) {
        v = new Date(`${v}T${c.op === "lte" ? "23:59:59.999" : "00:00:00"}`).toISOString();
      }
      return { field: c.field, [c.op]: v };
    });
  if (parts.length === 0) {
    return undefined;
  }
  return parts.length === 1 ? parts[0] : { and: parts };
}

// fromWhere reads a where back into conditions; null when it is more than AND of conditions
// (written by hand or by the API), which the list then shows as it is.
export function fromWhere(where: unknown): Condition[] | null {
  if (where === undefined || where === null) {
    return [];
  }
  const list = (where as { and?: unknown[] }).and ?? [where];
  const out: Condition[] = [];
  for (const node of list) {
    if (typeof node !== "object" || node === null || !("field" in node)) {
      return null;
    }
    const { field, ...rest } = node as Record<string, unknown>;
    const keys = Object.keys(rest);
    if (keys.length !== 1) {
      return null;
    }
    const op = keys[0] as Op;
    if (!["eq", "neq", "contains", "starts", "gte", "lte", "null"].includes(op)) {
      return null;
    }
    let value = op === "null" ? true : rest[op];
    // A moment shows as its day again.
    if (typeof value === "string" && /^\d{4}-\d{2}-\d{2}T/.test(value)) {
      const d = new Date(value);
      const pad = (n: number) => String(n).padStart(2, "0");
      value = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    }
    out.push({ field: String(field), op, value });
  }
  return out;
}

function ValueInput({
  id,
  subject,
  op,
  value,
  onChange,
}: {
  id: string;
  subject: Subject;
  op: Op;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const { t, i18n } = useTranslation();
  const label = t("filter.value");
  if (op === "null") {
    return null;
  }
  const select = (options: { value: string; label: string }[]) => (
    <select
      id={id}
      aria-label={label}
      className="h-10 rounded-md border border-input bg-background px-3 text-sm"
      value={String(value ?? "")}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="">{t("form.none")}</option>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
  if (subject.kind === "role" || (subject.kind === "field" && subject.field.type === "reference")) {
    const targets = subject.kind === "role" ? subject.to : [subject.field.to ?? ""];
    const v = String(value ?? "");
    const isWord = v.startsWith("@");
    return (
      <div className="flex min-w-60 flex-col gap-1">
        {subject.kind === "role" &&
          select([
            { value: "@me", label: t("filter.me") },
            { value: "@my_groups", label: t("filter.myGroups") },
          ])}
        {!isWord && (
          <RefPicker
            id={`${id}-pick`}
            targets={targets}
            single
            value={v ? [{ type: targets[0] ?? "", id: v } as ActorRef] : []}
            onChange={(refs) => onChange(refs[0]?.id ?? "")}
          />
        )}
      </div>
    );
  }
  if (subject.kind === "state") {
    return select(
      subject.states.map((st) => ({
        value: st.name,
        label: labelOf(st.label, i18n.language, st.name),
      })),
    );
  }
  if (subject.kind === "time") {
    return (
      <Input
        id={id}
        aria-label={label}
        type="date"
        value={String(value ?? "")}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  }
  const f = subject.field;
  switch (f.type) {
    case "choice":
      return select(
        (f.options ?? []).map((o) => ({
          value: o.value,
          label: optionLabel(f, o.value, i18n.language),
        })),
      );
    case "boolean":
      return (
        <select
          id={id}
          aria-label={label}
          className="h-10 rounded-md border border-input bg-background px-3 text-sm"
          value={value === true ? "true" : value === false ? "false" : ""}
          onChange={(e) => onChange(e.target.value === "" ? "" : e.target.value === "true")}
        >
          <option value="">{t("form.none")}</option>
          <option value="true">{t("filter.yes")}</option>
          <option value="false">{t("filter.no")}</option>
        </select>
      );
    case "number":
      return (
        <Input
          id={id}
          aria-label={label}
          type="number"
          step="any"
          value={value === undefined || value === "" ? "" : String(value)}
          onChange={(e) => onChange(e.target.value === "" ? "" : Number(e.target.value))}
        />
      );
    case "date":
    case "datetime":
      return (
        <Input
          id={id}
          aria-label={label}
          type="date"
          value={String(value ?? "")}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    default:
      return (
        <Input
          id={id}
          aria-label={label}
          value={String(value ?? "")}
          onChange={(e) => onChange(e.target.value)}
        />
      );
  }
}

export function Filters({
  type,
  conditions,
  onChange,
}: {
  type: TypeDefinition;
  conditions: Condition[];
  onChange: (c: Condition[]) => void;
}) {
  const { t, i18n } = useTranslation();
  const baseId = useId();
  const all = subjects(type);
  const subjectLabel = (s: Subject) => {
    switch (s.kind) {
      case "field":
        return fieldLabel(s.field, i18n.language);
      case "state":
        return t("column.state");
      case "time":
        return t(`column.${s.name}`);
      case "role":
        return labelOf(s.label, i18n.language, s.name);
    }
  };
  const set = (i: number, c: Condition) => onChange(conditions.map((x, j) => (j === i ? c : x)));
  const add = () => {
    const first = all[0]!;
    onChange([...conditions, { field: first.name, op: opsFor(first)[0]!, value: "" }]);
  };
  return (
    <fieldset className="flex flex-col gap-2 rounded-md border p-3">
      <legend className="px-1 text-sm font-semibold">{t("filter.title")}</legend>
      {conditions.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("filter.none")}</p>
      )}
      <ul className="flex flex-col gap-2">
        {conditions.map((c, i) => {
          const subject = all.find((s) => s.name === c.field) ?? all[0]!;
          const id = `${baseId}-${i}`;
          return (
            <li
              key={i}
              className="flex flex-wrap items-start gap-2"
              aria-label={t("filter.condition", { n: i + 1 })}
            >
              <select
                aria-label={t("filter.field")}
                className="h-10 rounded-md border border-input bg-background px-3 text-sm"
                value={c.field}
                onChange={(e) => {
                  const s = all.find((x) => x.name === e.target.value)!;
                  set(i, { field: s.name, op: opsFor(s)[0]!, value: "" });
                }}
              >
                {all.map((s) => (
                  <option key={s.name} value={s.name}>
                    {subjectLabel(s)}
                  </option>
                ))}
              </select>
              <select
                aria-label={t("filter.op")}
                className="h-10 rounded-md border border-input bg-background px-3 text-sm"
                value={c.op}
                onChange={(e) => set(i, { ...c, op: e.target.value as Op })}
              >
                {opsFor(subject).map((op) => (
                  <option key={op} value={op}>
                    {t(`filter.ops.${op}`)}
                  </option>
                ))}
              </select>
              <ValueInput
                id={id}
                subject={subject}
                op={c.op}
                value={c.value}
                onChange={(v) => set(i, { ...c, value: v })}
              />
              <Button
                variant="outline"
                size="sm"
                className="h-10"
                onClick={() => onChange(conditions.filter((_, j) => j !== i))}
              >
                {t("filter.remove")}
              </Button>
            </li>
          );
        })}
      </ul>
      <div>
        <Button variant="outline" size="sm" onClick={add}>
          {t("filter.add")}
        </Button>
      </div>
    </fieldset>
  );
}
