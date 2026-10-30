// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The conditions of a form (UI12, Hauptplan E134), as the server evaluates them in
// internal/forms: a section or question shows when all its conditions hold for the answers
// of the questions shown before it; a hidden question's answer does not count.
import type { FormDefinition } from "@/api/client";

type Condition = NonNullable<FormDefinition["sections"][number]["show_if"]>[number];

// answerText is an answer as a condition compares it.
export function answerText(v: unknown): string {
  if (v === null || v === undefined) {
    return "";
  }
  if (typeof v === "boolean") {
    return v ? "true" : "false";
  }
  if (typeof v === "number") {
    return Number.isNaN(v) ? "" : String(v);
  }
  return String(v);
}

export function holds(conditions: Condition[] | undefined, answers: Map<string, unknown>): boolean {
  for (const c of conditions ?? []) {
    const text = answerText(answers.get(c.question));
    const empty = text === "";
    switch (c.op) {
      case "answered":
        if (empty) return false;
        break;
      case "empty":
        if (!empty) return false;
        break;
      case "eq":
        if (empty || text !== c.value) return false;
        break;
      case "ne":
        if (!empty && text === c.value) return false;
        break;
    }
  }
  return true;
}

// hiddenOf names the questions (fields) and sections (groups) hidden for the values.
export function hiddenOf(
  def: FormDefinition,
  values: Record<string, unknown>,
): { fields: Set<string>; groups: Set<string> } {
  const fields = new Set<string>();
  const groups = new Set<string>();
  const shown = new Map<string, unknown>();
  for (const s of def.sections) {
    const visible = holds(s.show_if, shown);
    if (!visible) {
      groups.add(s.id);
    }
    for (const q of s.questions) {
      if (!visible || !holds(q.show_if, shown)) {
        fields.add(q.id);
        continue;
      }
      shown.set(q.id, values[q.id]);
    }
  }
  return { fields, groups };
}
