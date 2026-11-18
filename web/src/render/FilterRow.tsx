// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The filter row of a list (UI50, UI51, UI-Plan E54, E57; after the smartflow-client): one row
// with the filters as chips and a field to type in. A filter is typed in three steps, field,
// comparison and value, each with suggestions (WAI-ARIA combobox with a listbox); the
// comparison is chosen in advance, so "stat", Enter, "wart", Enter is a filter. Enter takes a
// suggestion, a comma takes it and waits for another value, arrow left goes to the comparison,
// the backspace in the empty field goes a step back (into the last filter, too), Esc closes
// the suggestions and then leaves the row. A filter works at once. A click on a chip changes
// it. Values come from the definition: choices, states, yes and no, records and roles (with
// "mir" and "meinen Gruppen", UI51), periods that run with the clock (kept as time words of
// the query, Kern-Plan PL72), and typed text, numbers and days. The conditions are those of
// Filters.tsx, so a saved search reads as before.
import { Filter, X } from "lucide-react";
import { useId, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { labelOf, type TypeDefinition } from "@/api/client";
import { cn } from "@/ui/utils";

import { optionLabel, fieldLabel } from "./fields";
import {
  fixedPeriods,
  opsFor,
  subjects,
  units,
  valueless,
  type Condition,
  type Op,
  type Subject,
  type Unit,
} from "./Filters";
import { RefName } from "./RefName";
import { useRefOptions } from "./RefPicker";

type Step = "field" | "op" | "value";
type Draft = { subject: Subject; op: Op; values: unknown[] };
type Suggestion = { id: string; label: string; hint?: string; value?: unknown; pick: () => void };

// The words of a typed period, German and English: "letzte 5 t", "last 5 days".
const lastWords = /^(letzte[nrs]?|vergangene[nr]?|last|past)$/;
const nextWords = /^(nächste[nrs]?|naechste[nrs]?|kommende[nr]?|next)$/;
const unitWords: Record<Unit, string[]> = {
  m: ["minuten", "minutes"],
  h: ["stunden", "std", "hours"],
  d: ["tage", "tagen", "days"],
  w: ["wochen", "weeks"],
  mo: ["monate", "monaten", "months"],
};

// typedPeriods reads "letzte 5 t" into the periods it can mean: one per fitting unit.
export function typedPeriods(text: string, withNext: boolean): string[] {
  const m = /^(\S+)\s*(\d{1,4})\s*(\S*)$/.exec(text.trim().toLowerCase());
  if (!m) {
    return [];
  }
  const dir = lastWords.test(m[1]!) ? "last" : withNext && nextWords.test(m[1]!) ? "next" : "";
  const n = Number(m[2]);
  if (!dir || n < 1) {
    return [];
  }
  const unit = m[3]!;
  return units
    .filter((u) => unit === "" || unitWords[u].some((w) => w.startsWith(unit)))
    .map((u) => `${dir}:${n}:${u}`);
}

// parseDay reads a day as 1.11.2026 or 2026-11-01.
export function parseDay(text: string): string | undefined {
  const s = text.trim();
  if (/^\d{4}-\d{2}-\d{2}$/.test(s)) {
    return s;
  }
  const m = /^(\d{1,2})\.(\d{1,2})\.(\d{4})$/.exec(s);
  if (!m) {
    return undefined;
  }
  const pad = (v: string) => v.padStart(2, "0");
  return `${m[3]}-${pad(m[2]!)}-${pad(m[1]!)}`;
}

function isTime(s: Subject) {
  return (
    s.kind === "time" ||
    s.kind === "deadline" ||
    (s.kind === "field" && (s.field.type === "date" || s.field.type === "datetime"))
  );
}

// refTargets are the types a value of the subject is a record of: a reference or a role.
function refTargets(s: Subject): string[] {
  if (s.kind === "role") {
    return s.to;
  }
  if (s.kind === "field" && s.field.type === "reference") {
    return [s.field.to ?? ""];
  }
  return [];
}

// several says whether a filter takes more than one value (one of them, or none of them).
function several(d: Draft) {
  if (d.op !== "eq" && d.op !== "neq") {
    return false;
  }
  return !isTime(d.subject) && !(d.subject.kind === "field" && d.subject.field.type === "boolean");
}

export function FilterRow({
  type,
  conditions,
  onChange,
}: {
  type: TypeDefinition;
  conditions: Condition[];
  onChange: (conditions: Condition[]) => void;
}) {
  const { t, i18n } = useTranslation();
  const lang = i18n.language;
  const all = useMemo(() => subjects(type), [type]);
  const input = useRef<HTMLInputElement>(null);
  const listId = useId();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [step, setStep] = useState<Step>("field");
  const [text, setText] = useState("");
  const [active, setActive] = useState(0);
  const [open, setOpen] = useState(false);
  // The names of records chosen here, so that their chips need not ask the server.
  const [names, setNames] = useState<Record<string, string>>({});
  const targets = draft && step === "value" ? refTargets(draft.subject) : [];
  const refs = useRefOptions(targets, text, open && targets.length > 0);

  const subjectName = (s: Subject) => {
    switch (s.kind) {
      case "field":
        return fieldLabel(s.field, lang);
      case "state":
        return t("column.state");
      case "time":
      case "deadline":
        return t(`column.${s.name}`);
      case "role":
        return labelOf(s.label, lang, s.name);
    }
  };
  const opName = (s: Subject, op: Op) =>
    s.kind === "deadline" && op === "lte" ? t("filter.ops.dueBy") : t(`filter.ops.${op}`);
  const periodName = (p: string) => {
    if ((fixedPeriods as readonly string[]).includes(p)) {
      return t(`filter.period.${p}`);
    }
    const m = /^(last|next):(\d+):(mo|m|h|d|w)$/.exec(p);
    if (!m) {
      return p;
    }
    const count = Number(m[2]);
    return t(`filter.period.${m[1]}`, { n: count, unit: t(`filter.unit.${m[3]}`, { count }) });
  };
  const dayName = (day: string) => {
    const d = new Date(`${day}T00:00:00`);
    return Number.isNaN(d.getTime()) ? day : new Intl.DateTimeFormat(lang).format(d);
  };
  const valueName = (s: Subject, op: Op, v: unknown): ReactNode => {
    if (op === "in") {
      return periodName(String(v));
    }
    if (v === "@me") {
      return t("filter.toMe");
    }
    if (v === "@my_groups") {
      return t("filter.toMyGroups");
    }
    if (s.kind === "state") {
      const st = s.states.find((x) => x.name === v);
      return st ? labelOf(st.label, lang, st.name) : String(v);
    }
    const to = refTargets(s);
    if (to.length > 0) {
      const id = String(v);
      return names[id] ?? <RefName refTo={{ type: to[0]!, id }} />;
    }
    if (s.kind === "field") {
      if (s.field.type === "choice") {
        return optionLabel(s.field, String(v), lang);
      }
      if (s.field.type === "boolean") {
        return v === true ? t("filter.yes") : t("filter.no");
      }
    }
    if (typeof v === "string" && /^\d{4}-\d{2}-\d{2}$/.test(v)) {
      return dayName(v);
    }
    return String(v);
  };

  const reset = () => {
    setDraft(null);
    setStep("field");
    setText("");
    setActive(0);
  };
  const commit = (d: Draft, values: unknown[]) => {
    onChange([
      ...conditions,
      { field: d.subject.name, op: d.op, value: values.length === 1 ? values[0] : values },
    ]);
    reset();
  };
  const takeValue = (v: unknown, more: boolean) => {
    if (!draft) {
      return;
    }
    const values = [...draft.values, v];
    if (more && several(draft)) {
      setDraft({ ...draft, values });
      setText("");
      setActive(0);
      return;
    }
    commit(draft, values);
  };
  const takeOp = (d: Draft, op: Op) => {
    if (valueless.includes(op)) {
      commit({ ...d, op }, [""]);
      return;
    }
    setDraft({ ...d, op, values: [] });
    setStep("value");
    setText("");
    setActive(0);
  };
  const takeSubject = (s: Subject) => {
    setDraft({ subject: s, op: opsFor(s)[0]!, values: [] });
    setStep("value");
    setText("");
    setActive(0);
  };
  // edit takes a filter back into the row, to change it; its chip goes.
  const edit = (i: number) => {
    const c = conditions[i];
    const s = all.find((x) => x.name === c?.field);
    if (!c || !s) {
      return;
    }
    onChange(conditions.filter((_, j) => j !== i));
    const values = valueless.includes(c.op) ? [] : Array.isArray(c.value) ? c.value : [c.value];
    setDraft({ subject: s, op: c.op, values });
    setStep(valueless.includes(c.op) ? "op" : "value");
    setText("");
    setActive(0);
    setOpen(true);
    input.current?.focus();
  };

  const needle = text.trim().toLowerCase();
  const has = (label: string) => needle === "" || label.toLowerCase().includes(needle);
  // rank puts what starts with the text first, then what has a word starting with it: "art"
  // finds "Art" before "Wartegrund".
  const rank = (label: string) => {
    const l = label.toLowerCase();
    return l.startsWith(needle) ? 0 : l.split(/[\s(/-]+/).some((w) => w.startsWith(needle)) ? 1 : 2;
  };
  const byRank = <T,>(list: T[], label: (x: T) => string) =>
    needle === "" ? list : [...list].sort((a, b) => rank(label(a)) - rank(label(b)));
  const suggestions: Suggestion[] = (() => {
    if (step === "field" || !draft) {
      return byRank(
        all.filter((s) => has(subjectName(s))),
        subjectName,
      ).map((s) => ({
        id: `f-${s.name}`,
        label: subjectName(s),
        hint: t(`filter.kind.${s.kind === "field" ? s.field.type : s.kind}`, {
          defaultValue: "",
        }),
        pick: () => takeSubject(s),
      }));
    }
    const s = draft.subject;
    if (step === "op") {
      return opsFor(s)
        .filter((op) => has(opName(s, op)))
        .map((op) => ({ id: `o-${op}`, label: opName(s, op), pick: () => takeOp(draft, op) }));
    }
    const value = (id: string, label: string, v: unknown, hint?: string): Suggestion => ({
      id: `v-${id}`,
      label,
      hint,
      value: v,
      pick: () => takeValue(v, false),
    });
    if (draft.op === "in") {
      const deadline = s.kind === "deadline";
      return [
        ...typedPeriods(text, deadline).map((p) => value(p, periodName(p), p)),
        ...fixedPeriods.filter((p) => has(periodName(p))).map((p) => value(p, periodName(p), p)),
      ];
    }
    if (isTime(s)) {
      const day = parseDay(text);
      return day ? [value(day, dayName(day), day)] : [];
    }
    if (s.kind === "state") {
      return s.states
        .map((st) => ({ name: st.name, label: labelOf(st.label, lang, st.name) }))
        .filter((st) => has(st.label))
        .sort((x, y) => rank(x.label) - rank(y.label))
        .map((st) => value(st.name, st.label, st.name));
    }
    const to = refTargets(s);
    if (to.length > 0) {
      const words =
        s.kind === "role"
          ? [
              value("@me", t("filter.toMe"), "@me"),
              value("@my_groups", t("filter.toMyGroups"), "@my_groups"),
            ].filter((x) => has(x.label))
          : [];
      return [
        ...words,
        ...refs.options.map((o) => ({
          ...value(o.id, o.title, o.id, refs.typeLabel(o.type, lang)),
          pick: () => {
            setNames((n) => ({ ...n, [o.id]: o.title }));
            takeValue(o.id, false);
          },
        })),
      ];
    }
    if (s.kind !== "field") {
      return [];
    }
    const f = s.field;
    if (f.type === "choice") {
      return (f.options ?? [])
        .map((o) => ({ value: o.value, label: optionLabel(f, o.value, lang) }))
        .filter((o) => has(o.label))
        .sort((x, y) => rank(x.label) - rank(y.label))
        .map((o) => value(o.value, o.label, o.value));
    }
    if (f.type === "boolean") {
      return [value("true", t("filter.yes"), true), value("false", t("filter.no"), false)].filter(
        (x) => has(x.label),
      );
    }
    if (text.trim() === "") {
      return [];
    }
    if (f.type === "number") {
      const n = Number(text.trim().replace(",", "."));
      return Number.isNaN(n) ? [] : [value("n", String(n), n, t("filter.take"))];
    }
    return [value("t", `„${text.trim()}“`, text.trim(), t("filter.take"))];
  })();
  // Records a value of a reference may be: also keep their names for the chips.
  const choice = suggestions[Math.min(active, suggestions.length - 1)];

  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setOpen(true);
        setActive((a) => (suggestions.length ? (a + 1) % suggestions.length : 0));
        return;
      case "ArrowUp":
        e.preventDefault();
        setActive((a) =>
          suggestions.length ? (a - 1 + suggestions.length) % suggestions.length : 0,
        );
        return;
      case "Enter":
        e.preventDefault();
        // Values taken with a comma, and nothing typed: Enter ends the filter with them.
        if (draft && step === "value" && draft.values.length > 0 && text === "") {
          commit(draft, draft.values);
        } else if (choice) {
          choice.pick();
        }
        return;
      case ",":
        if (draft && step === "value" && several(draft)) {
          e.preventDefault();
          if (choice?.value !== undefined) {
            if (refTargets(draft.subject).length > 0 && typeof choice.value === "string") {
              setNames((n) => ({ ...n, [String(choice.value)]: choice.label }));
            }
            takeValue(choice.value, true);
          }
        }
        return;
      case "ArrowLeft":
        if (draft && step === "value" && text === "") {
          e.preventDefault();
          setStep("op");
          setActive(0);
        }
        return;
      case "Backspace":
        if (text !== "") {
          return;
        }
        e.preventDefault();
        if (draft && step === "value") {
          if (draft.values.length > 0) {
            setDraft({ ...draft, values: draft.values.slice(0, -1) });
          } else {
            setStep("op");
          }
        } else if (draft && step === "op") {
          reset();
        } else if (conditions.length > 0) {
          edit(conditions.length - 1);
        }
        return;
      case "Escape":
        e.preventDefault();
        if (open && (draft || text)) {
          setOpen(false);
          return;
        }
        reset();
        setOpen(false);
        input.current?.blur();
        return;
    }
  };

  const caption =
    step === "field" ? t("filter.field") : step === "op" ? t("filter.op") : t("filter.value");
  const placeholder =
    step === "field"
      ? conditions.length === 0
        ? t("filter.placeholder")
        : t("filter.placeholderMore")
      : step === "op"
        ? t("filter.pickOp")
        : t("filter.pickValue");
  const optionId = (s: Suggestion) => `${listId}-${s.id}`;
  const showList = open;

  return (
    <div className="relative flex min-h-8 min-w-60 flex-1 flex-wrap items-center gap-1 rounded-md border border-input bg-card px-2 py-0.5 focus-within:ring-2 focus-within:ring-ring">
      <Filter aria-hidden="true" className="mr-0.5 size-4 flex-none text-muted-foreground" />
      <ul aria-label={t("filter.title")} className="contents">
        {conditions.map((c, i) => {
          const s = all.find((x) => x.name === c.field);
          const label = s ? subjectName(s) : c.field;
          const values = valueless.includes(c.op)
            ? []
            : Array.isArray(c.value)
              ? c.value
              : [c.value];
          return (
            <li
              key={i}
              className="inline-flex h-6 items-center rounded-full border border-input bg-muted text-[13px]"
            >
              <button
                type="button"
                title={t("filter.change")}
                onClick={() => edit(i)}
                className="inline-flex h-6 items-center gap-1 rounded-l-full pr-0.5 pl-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="text-foreground-2">
                  {label}
                  {c.op === "in" ? ":" : ""}
                </span>{" "}
                {c.op !== "in" && s && (
                  <>
                    <span className="text-foreground-2">{opName(s, c.op)}</span>{" "}
                  </>
                )}
                {s &&
                  values.map((v, j) => (
                    <span key={j} className="font-medium">
                      {j > 0 && <span className="font-normal text-foreground-2">, </span>}
                      {valueName(s, c.op, v)}
                    </span>
                  ))}
              </button>
              <button
                type="button"
                aria-label={t("filter.removeOne", { name: label })}
                onClick={() => onChange(conditions.filter((_, j) => j !== i))}
                className="inline-grid h-6 w-6 place-items-center rounded-r-full text-foreground-2 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <X aria-hidden="true" className="size-3" strokeWidth={2.5} />
              </button>
            </li>
          );
        })}
      </ul>
      {draft && (
        <span className="inline-flex h-6 items-center gap-1 rounded-full border border-dashed border-input px-2 text-[13px]">
          <span className="text-foreground-2">{subjectName(draft.subject)}</span>
          <span
            className={cn(
              step === "op" ? "font-semibold text-foreground" : "text-muted-foreground",
            )}
          >
            {draft.op === "in" ? ":" : opName(draft.subject, draft.op)}
          </span>
          {draft.values.map((v, j) => (
            <span key={j} className="font-medium">
              {j > 0 && <span className="font-normal text-foreground-2">, </span>}
              {valueName(draft.subject, draft.op, v)}
            </span>
          ))}
        </span>
      )}
      <input
        ref={input}
        data-filter-input=""
        type="text"
        role="combobox"
        aria-label={t("filter.set")}
        aria-autocomplete="list"
        aria-expanded={showList}
        aria-controls={listId}
        aria-activedescendant={showList && choice ? optionId(choice) : undefined}
        autoComplete="off"
        value={text}
        placeholder={placeholder}
        onChange={(e) => {
          setText(e.target.value);
          setActive(0);
          setOpen(true);
        }}
        onKeyDown={onKey}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        className="h-7 min-w-32 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
      />
      <kbd
        aria-hidden="true"
        title={t("filter.slash")}
        className="hidden rounded border border-b-2 border-input bg-card px-1.5 font-mono text-xs text-foreground-2 sm:inline"
      >
        /
      </kbd>
      {showList && (
        <div className="absolute top-full left-0 z-50 mt-1 w-88 max-w-[calc(100vw-2rem)] rounded-md border border-input bg-card p-1.5 shadow-lg">
          <p className="px-2 pt-1 pb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {caption}
          </p>
          <ul
            id={listId}
            role="listbox"
            aria-label={caption}
            className="flex max-h-72 flex-col overflow-y-auto"
          >
            {suggestions.map((s) => (
              <li
                key={s.id}
                id={optionId(s)}
                role="option"
                aria-selected={s === choice}
                onMouseDown={(e) => {
                  e.preventDefault();
                  s.pick();
                }}
                className={cn(
                  "flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm",
                  s === choice && "bg-surface-active",
                )}
              >
                <span className="flex-1">{s.label}</span>
                {s.hint && <span className="text-xs text-foreground-2">{s.hint}</span>}
              </li>
            ))}
          </ul>
          {suggestions.length === 0 && (
            <p className="px-2 py-1.5 text-sm text-muted-foreground">
              {step === "value" && draft && isTime(draft.subject)
                ? t("filter.typeDay")
                : step === "value" && draft && refTargets(draft.subject).length > 0
                  ? t("filter.typeName")
                  : step === "value"
                    ? t("filter.typeValue")
                    : t("filter.nothing")}
            </p>
          )}
          <p className="mt-1 flex flex-wrap gap-x-3 gap-y-1 border-t border-border px-2 pt-1.5 text-xs text-foreground-2">
            <Hint k="Enter">{t("filter.keys.enter")}</Hint>
            {draft && step === "value" && several(draft) && (
              <Hint k=",">{t("filter.keys.comma")}</Hint>
            )}
            {draft && step === "value" && <Hint k="←">{t("filter.keys.left")}</Hint>}
            <Hint k="⌫">{t("filter.keys.back")}</Hint>
            <Hint k="Esc">{t("filter.keys.esc")}</Hint>
          </p>
        </div>
      )}
    </div>
  );
}

function Hint({ k, children }: { k: string; children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1">
      <kbd className="rounded border border-input px-1 font-mono text-[11px]">{k}</kbd>
      {children}
    </span>
  );
}
