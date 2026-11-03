// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// What a field type means for list and form (architektur.md §10: almost everything is
// rendered from the definition). The server validates again; the rules here only give the
// user a quick answer (Plattform-Kern PL3 defines them).
import { z } from "zod";

import {
  labelOf,
  type Field,
  type RecordData,
  type State,
  type TypeDefinition,
} from "@/api/client";
import type { StateCategory } from "@/ui/marks";

// System columns the list can show; sortable are the ones the API orders by key (PL4, E12).
export const systemColumns = [
  { name: "id", sortable: true },
  { name: "version", sortable: true },
  { name: "state", sortable: false },
  { name: "created_at", sortable: true },
  { name: "updated_at", sortable: true },
  // The next deadline of the SLA clocks (UI34), with its three-quarter point and pause.
  { name: "sla_due_at", sortable: true },
] as const;
export type SystemColumn = (typeof systemColumns)[number]["name"];

export const sortableColumns: readonly string[] = systemColumns
  .filter((c) => c.sortable)
  .map((c) => c.name);

// isCalculated tells the fields the server computes (sums of the costs, formulas, running
// numbers): the form shows them read-only and never sends them.
export function isCalculated(field: Field): boolean {
  return field.computed === true || (field.formula ?? "") !== "" || field.type === "sequence";
}

export function fieldLabel(field: Field, language: string): string {
  return labelOf(field.label, language, field.name);
}

export function optionLabel(field: Field, value: unknown, language: string): string {
  const option = field.options?.find((o) => o.value === value);
  return option ? labelOf(option.label, language, option.value) : String(value ?? "");
}

// The time zone moments are shown in (UI9): the person's setting, set by the frame; until
// then the browser's.
let displayTimeZone: string | undefined;

export function setDisplayTimeZone(tz: string | undefined): void {
  displayTimeZone = tz;
}

export function currentTimeZone(): string {
  return displayTimeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone;
}

// formatMoment shows a moment in the language and the time zone of the person.
export function formatMoment(value: unknown, language: string): string {
  const d = new Date(String(value));
  return displayTimeZone
    ? d.toLocaleString(language, { timeZone: displayTimeZone })
    : d.toLocaleString(language);
}

// plainText is the text of HTML without markup (UI7), for lists.
export function plainText(html: string): string {
  const doc = new DOMParser().parseFromString(html, "text/html");
  return (doc.body.textContent ?? "").replace(/\s+/g, " ").trim();
}

// stateCategory is the kind of a state for its mark (UI29): as the type says, else open for
// the initial state and active for the others.
export function stateCategory(state: State | undefined): StateCategory {
  return state?.category ?? (state?.initial ? "open" : "active");
}

// formatValue renders a value for the list.
export function formatValue(field: Field | undefined, value: unknown, language: string): string {
  if (value === null || value === undefined || value === "") {
    return "";
  }
  switch (field?.type) {
    case "boolean":
      return value ? "✓" : "–";
    case "choice":
      return optionLabel(field, value, language);
    case "datetime":
      return formatMoment(value, language);
    case "text":
      // Formatted text (UI7) as its plain text in lists and conflict views.
      return field.format === "rich" ? plainText(String(value)) : String(value);
    case "date":
      return new Date(String(value) + "T00:00:00").toLocaleDateString(language);
    case "sequence":
      // The running number with its prefix (PL49): PRB-24817.
      return `${field.prefix ?? ""}${String(value)}`;
    default:
      return typeof value === "object" ? JSON.stringify(value) : String(value);
  }
}

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// schemaFor builds the zod schema of a form from the fields; messages are codes that i18n
// turns into text (form.error.<code>).
export function schemaFor(type: TypeDefinition) {
  const shape: Record<string, z.ZodTypeAny> = {};
  for (const f of type.fields) {
    if (isCalculated(f)) {
      // Shown, never checked or sent (PL20, PL22).
      // Optional: a formula with empty input gives no value (PL22).
      shape[f.name] = z.unknown().optional();
      continue;
    }
    let s: z.ZodTypeAny;
    switch (f.type) {
      case "number": {
        let n = z.number({ error: "number" });
        if (f.min !== undefined) {
          n = n.min(f.min, "min");
        }
        if (f.max !== undefined) {
          n = n.max(f.max, "max");
        }
        // Empty input arrives as NaN from valueAsNumber.
        s = z.preprocess(
          (v) => (typeof v === "number" && Number.isNaN(v) ? undefined : v),
          f.required ? n : n.optional(),
        );
        break;
      }
      case "boolean":
        s = z.boolean();
        break;
      case "choice": {
        const values = (f.options ?? []).map((o) => o.value);
        const c =
          values.length > 0
            ? z.enum(values as [string, ...string[]], { error: "choice" })
            : z.string();
        s = f.required ? c : c.or(z.literal("")).optional();
        break;
      }
      case "reference": {
        const r = z.string().regex(uuid, "uuid");
        s = f.required ? r : r.or(z.literal("")).optional();
        break;
      }
      case "date":
      case "datetime": {
        const d = z.string().min(1, "required");
        s = f.required ? d : z.string().optional();
        break;
      }
      default: {
        let t = z.string();
        if (f.max_length) {
          t = t.max(f.max_length, "max_length");
        }
        s = f.required ? t.trim().min(1, "required") : t.optional();
      }
    }
    shape[f.name] = s;
  }
  return z.object(shape);
}

// toInput turns a record into form values: null becomes "" or false, datetimes lose the zone
// for datetime-local.
export function toInput(
  type: TypeDefinition,
  record: RecordData | undefined,
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of type.fields) {
    const v = record?.[f.name] ?? (record === undefined ? f.default : undefined);
    switch (f.type) {
      case "boolean":
        out[f.name] = v === true;
        break;
      case "number":
        out[f.name] = typeof v === "number" ? v : undefined;
        break;
      case "datetime":
        out[f.name] = typeof v === "string" && v !== "" ? toLocalInput(v) : "";
        break;
      case "sequence":
        // Shown read-only with its prefix; a new record gets it when it is saved.
        out[f.name] = v === null || v === undefined ? "" : `${f.prefix ?? ""}${String(v)}`;
        break;
      default:
        out[f.name] = v === null || v === undefined ? "" : String(v);
    }
  }
  return out;
}

// toRecord turns form values into the body of the API: "" becomes null, datetime-local gets
// its zone back.
export function toRecord(type: TypeDefinition, values: Record<string, unknown>): RecordData {
  const out: RecordData = {};
  for (const f of type.fields) {
    if (isCalculated(f)) {
      continue;
    }
    const v = values[f.name];
    switch (f.type) {
      case "boolean":
        out[f.name] = v === true;
        break;
      case "number":
        out[f.name] = typeof v === "number" && !Number.isNaN(v) ? v : null;
        break;
      case "datetime":
        out[f.name] = typeof v === "string" && v !== "" ? new Date(v).toISOString() : null;
        break;
      default:
        out[f.name] = v === "" || v === undefined ? null : v;
    }
  }
  return out;
}

function toLocalInput(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return "";
  }
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// changedFields lists the fields whose value differs between two records (for the conflict
// view, E29).
export function changedFields(type: TypeDefinition, a: RecordData, b: RecordData): Field[] {
  return type.fields.filter(
    (f) =>
      !isCalculated(f) && JSON.stringify(a[f.name] ?? null) !== JSON.stringify(b[f.name] ?? null),
  );
}
