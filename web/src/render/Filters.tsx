// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The filters of a list (UI4, Hauptplan E124): conditions "field – comparison – value", all
// joined with AND, sent as the where of the JSON query (E40). The comparisons follow the kind
// of the field; references and roles are chosen with the picker (UI28), a role also as "me" or
// "my groups". The deadline (UI34) is breached, close or due by a day, against the moment of
// the query ("@now"), so that a saved search stays right.
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { labelOf, type ActorRef, type Field, type State, type TypeDefinition } from "@/api/client";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";

import { fieldLabel, optionLabel } from "./fields";
import { RefPicker } from "./RefPicker";
import { Select } from "@/ui/select";

export type Op =
  "eq" | "neq" | "contains" | "starts" | "gte" | "lte" | "null" | "breached" | "close" | "in";
// value is one value, or a list for several (eq and neq become in and nin, UI50); with op
// "in" it is a period that runs with the clock (UI51, UI-Plan E57), see Period.
export type Condition = { field: string; op: Op; value: unknown };

// A filterable thing of the type: a field, the state, a system time or a role.
export type Subject =
  | { kind: "field"; name: string; field: Field }
  | { kind: "state"; name: "state"; states: State[] }
  | { kind: "time"; name: "created_at" | "updated_at" }
  | { kind: "deadline"; name: "sla_due_at" }
  | { kind: "role"; name: string; to: string[]; label?: Record<string, string> };

export function subjects(type: TypeDefinition): Subject[] {
  return [
    ...type.fields.map<Subject>((f) => ({ kind: "field", name: f.name, field: f })),
    ...(type.states?.length
      ? [{ kind: "state", name: "state", states: type.states } as const]
      : []),
    { kind: "time", name: "created_at" },
    { kind: "time", name: "updated_at" },
    ...(type.timed ? [{ kind: "deadline", name: "sla_due_at" } as const] : []),
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
      // A period first (UI51), then from and until a day.
      return ["in", "gte", "lte"];
    case "deadline":
      return ["in", "breached", "close", "lte"];
    case "role":
      return ["eq", "null"];
  }
  switch (s.field.type) {
    case "text":
      return ["contains", "eq", "starts", "null"];
    case "number":
      return ["eq", "gte", "lte", "null"];
    case "date":
      return ["in", "eq", "gte", "lte", "null"];
    case "sequence":
      // The running number, with or without its prefix (PL49).
      return ["eq", "gte", "lte"];
    case "datetime":
      return ["in", "gte", "lte", "null"];
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
    field === "sla_due_at" ||
    type.fields.some((f) => f.name === field && f.type === "datetime")
  );
}

// valueless are the comparisons without a value.
export const valueless: Op[] = ["null", "breached", "close"];

// --- periods that run with the clock (UI51, UI-Plan E57, Kern-Plan PL72) ---
//
// A period is kept as a word of the JSON query, never as a fixed day, so that a saved search
// shows "today" again tomorrow: "today" is gte @today, "the last 5 days" gte @now-5d (exactly
// 5 × 24 hours back, your decision V13), "last month" gte @month-1mo and lt @month. As a
// condition value it is one of the names below, or last:<n>:<unit> and next:<n>:<unit> (the
// next ones at a deadline: "due in the next 2 days").
export const fixedPeriods = [
  "today",
  "yesterday",
  "this_week",
  "this_month",
  "this_year",
  "last_week",
  "last_month",
] as const;
export type Unit = "m" | "h" | "d" | "w" | "mo";
export const units: Unit[] = ["m", "h", "d", "w", "mo"];

// periodNodes are the conditions of a period on a field; none for a value that is no period.
export function periodNodes(field: string, period: string): Record<string, unknown>[] {
  const fixed: Record<string, [string, string?]> = {
    today: ["@today"],
    yesterday: ["@today-1d", "@today"],
    this_week: ["@week"],
    this_month: ["@month"],
    this_year: ["@year"],
    last_week: ["@week-1w", "@week"],
    last_month: ["@month-1mo", "@month"],
  };
  const f = fixed[period];
  if (f) {
    return f[1]
      ? [
          { field, gte: f[0] },
          { field, lt: f[1] },
        ]
      : [{ field, gte: f[0] }];
  }
  const m = /^(last|next):(\d+):(mo|m|h|d|w)$/.exec(period);
  if (!m) {
    return [];
  }
  return m[1] === "last"
    ? [{ field, gte: `@now-${m[2]}${m[3]}` }]
    : [
        { field, gte: "@now" },
        { field, lte: `@now+${m[2]}${m[3]}` },
      ];
}

// readPeriods takes the conditions of periods out of a list of where nodes: the periods by
// field, and the nodes that are left.
function readPeriods(list: unknown[]): { periods: Condition[]; rest: unknown[] } {
  const node = (n: unknown) =>
    typeof n === "object" && n !== null ? (n as Record<string, unknown>) : undefined;
  const used = new Set<number>();
  const periods: Condition[] = [];
  const find = (field: unknown, op: string, value: string) =>
    list.findIndex(
      (n, i) =>
        !used.has(i) &&
        node(n)?.["field"] === field &&
        node(n)?.[op] === value &&
        Object.keys(node(n)!).length === 2,
    );
  list.forEach((n, i) => {
    const x = node(n);
    if (used.has(i) || !x || Object.keys(x).length !== 2 || typeof x["gte"] !== "string") {
      return;
    }
    const field = x["field"];
    const from = x["gte"];
    const pairs: Record<string, [string, string]> = {
      "@today-1d": ["@today", "yesterday"],
      "@week-1w": ["@week", "last_week"],
      "@month-1mo": ["@month", "last_month"],
    };
    const pair = pairs[from];
    if (pair) {
      const j = find(field, "lt", pair[0]);
      if (j >= 0) {
        used.add(i).add(j);
        periods.push({ field: String(field), op: "in", value: pair[1] });
      }
      return;
    }
    const single: Record<string, string> = {
      "@today": "today",
      "@week": "this_week",
      "@month": "this_month",
      "@year": "this_year",
    };
    if (single[from]) {
      used.add(i);
      periods.push({ field: String(field), op: "in", value: single[from] });
      return;
    }
    const last = /^@now-(\d+)(mo|m|h|d|w)$/.exec(from);
    if (last) {
      used.add(i);
      periods.push({ field: String(field), op: "in", value: `last:${last[1]}:${last[2]}` });
      return;
    }
    if (from === "@now") {
      const j = list.findIndex((m, k) => {
        const y = node(m);
        if (!y || used.has(k) || k === i) {
          return false;
        }
        const until = y["lte"];
        return (
          y["field"] === field &&
          typeof until === "string" &&
          /^@now\+\d+(mo|m|h|d|w)$/.test(until) &&
          Object.keys(y).length === 2
        );
      });
      if (j >= 0) {
        const next = /^@now\+(\d+)(mo|m|h|d|w)$/.exec(String(node(list[j])!["lte"]))!;
        used.add(i).add(j);
        periods.push({ field: String(field), op: "in", value: `next:${next[1]}:${next[2]}` });
      }
    }
  });
  return { periods, rest: list.filter((_, i) => !used.has(i)) };
}

// toWhere turns the conditions into the where of the JSON query; incomplete ones are left out.
export function toWhere(
  type: TypeDefinition,
  conditions: Condition[],
): Record<string, unknown> | undefined {
  const parts = conditions
    .filter(
      (c) =>
        valueless.includes(c.op) || (c.value !== "" && c.value !== undefined && c.value !== null),
    )
    .flatMap((c) => {
      if (c.op === "null") {
        return [{ field: c.field, null: true }];
      }
      if (c.op === "breached") {
        return [{ field: "sla_due_at", lte: "@now" }];
      }
      if (c.op === "close") {
        return [
          { field: "sla_warn_at", lte: "@now" },
          { field: "sla_due_at", gt: "@now" },
        ];
      }
      if (c.op === "in") {
        return periodNodes(c.field, String(c.value));
      }
      // Several values: one of them, or none of them (UI50).
      if (Array.isArray(c.value)) {
        if (c.value.length === 0) {
          return [];
        }
        if (c.value.length > 1 && (c.op === "eq" || c.op === "neq")) {
          return [{ field: c.field, [c.op === "eq" ? "in" : "nin"]: c.value }];
        }
      }
      let v = Array.isArray(c.value) ? c.value[0] : c.value;
      if (isMoment(type, c.field) && typeof v === "string" && /^\d{4}-\d{2}-\d{2}$/.test(v)) {
        v = new Date(`${v}T${c.op === "lte" ? "23:59:59.999" : "00:00:00"}`).toISOString();
      }
      return [{ field: c.field, [c.op]: v }];
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
  const read = readPeriods((where as { and?: unknown[] }).and ?? [where]);
  const all = read.rest;
  // The deadline against "@now" reads back as breached or close (UI34).
  const isNode = (n: unknown, field: string, op: string) =>
    typeof n === "object" &&
    n !== null &&
    (n as Record<string, unknown>)["field"] === field &&
    (n as Record<string, unknown>)[op] === "@now";
  const close = all.some((n) => isNode(n, "sla_warn_at", "lte"));
  const list = all.filter(
    (n) => !(close && (isNode(n, "sla_warn_at", "lte") || isNode(n, "sla_due_at", "gt"))),
  );
  const out: Condition[] = [...read.periods];
  if (close) {
    out.push({ field: "sla_due_at", op: "close", value: "" });
  }
  for (const node of list) {
    if (typeof node !== "object" || node === null || !("field" in node)) {
      return null;
    }
    const { field, ...rest } = node as Record<string, unknown>;
    const keys = Object.keys(rest);
    if (keys.length !== 1) {
      return null;
    }
    const key = keys[0]!;
    // Several values read back as one condition with a list (UI50).
    if ((key === "in" || key === "nin") && Array.isArray(rest[key])) {
      out.push({ field: String(field), op: key === "in" ? "eq" : "neq", value: rest[key] });
      continue;
    }
    const op = key as Op;
    if (!["eq", "neq", "contains", "starts", "gte", "lte", "null"].includes(op)) {
      return null;
    }
    if (field === "sla_due_at" && op === "lte" && rest[op] === "@now") {
      out.push({ field: "sla_due_at", op: "breached", value: "" });
      continue;
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
  if (valueless.includes(op)) {
    return null;
  }
  const select = (options: { value: string; label: string }[]) => (
    <Select
      id={id}
      label={label}
      value={String(value ?? "")}
      onChange={onChange}
      options={[{ value: "", label: t("form.none") }, ...options]}
    />
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
  if (subject.kind === "time" || subject.kind === "deadline") {
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
        <Select
          id={id}
          label={label}
          value={value === true ? "true" : value === false ? "false" : ""}
          onChange={(v) => onChange(v === "" ? "" : v === "true")}
          options={[
            { value: "", label: t("form.none") },
            { value: "true", label: t("filter.yes") },
            { value: "false", label: t("filter.no") },
          ]}
        />
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
      case "deadline":
        return t(`column.${s.name}`);
      case "role":
        return labelOf(s.label, i18n.language, s.name);
    }
  };
  const set = (i: number, c: Condition) => onChange(conditions.map((x, j) => (j === i ? c : x)));
  const add = () => {
    const first = all[0]!;
    onChange([
      ...conditions,
      { field: first.name, op: opsFor(first).find((o) => o !== "in")!, value: "" },
    ]);
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
              <Select
                label={t("filter.field")}
                value={c.field}
                onChange={(v) => {
                  const s = all.find((x) => x.name === v)!;
                  set(i, { field: s.name, op: opsFor(s).find((o) => o !== "in")!, value: "" });
                }}
                options={all.map((s) => ({ value: s.name, label: subjectLabel(s) }))}
              />
              <Select
                label={t("filter.op")}
                value={c.op}
                onChange={(v) => set(i, { ...c, op: v as Op })}
                options={opsFor(subject)
                  .filter((op) => op !== "in")
                  .map((op) => ({
                    value: op,
                    label:
                      subject.kind === "deadline" && op === "lte"
                        ? t("filter.ops.dueBy")
                        : t(`filter.ops.${op}`),
                  }))}
              />
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
