// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The form builder (UI11, Hauptplan E133): the editor of the field definition of core.form.
// Sections with questions; a question is bound to a field of the target type or free with a
// field type of the core. Sections and questions are ordered by dragging with dnd-kit, with
// the mouse or the keyboard (space picks up, arrows move, space drops), and with the buttons
// "up" and "down". The value is the JSON the server checks when the form is saved. A switch
// puts every text of the structure into German or English (UI12, E134); sections and questions
// carry conditions on earlier questions ("show only when").
import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type Announcements,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useQuery } from "@tanstack/react-query";
import { useId, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  labelOf,
  qualifiedName,
  typesQuery,
  type Field,
  type FormDefinition,
  type FormQuestion,
  type FormSection,
  type Label as Text,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";

import type { EditorProps } from "./Form";
import { fieldLabel, isCalculated } from "./fields";

// freeTypes are the types of a free question (the server allows the same).
export const freeTypes = ["text", "number", "boolean", "date", "datetime", "choice"] as const;

// parse reads the value of the field; an empty or broken one starts with one section.
function parse(value: unknown): { def: FormDefinition; broken: boolean } {
  const fresh = { sections: [{ id: "s1", title: { de: "" }, questions: [] }] };
  if (typeof value !== "string" || value.trim() === "") {
    return { def: fresh, broken: false };
  }
  try {
    const def = JSON.parse(value) as FormDefinition;
    if (!Array.isArray(def.sections)) {
      return { def: fresh, broken: true };
    }
    return { def, broken: false };
  } catch {
    return { def: fresh, broken: true };
  }
}

// nextId is the first id with the prefix that the definition does not use yet.
function nextId(def: FormDefinition, prefix: string): string {
  const used = new Set(def.sections.flatMap((s) => [s.id, ...s.questions.map((q) => q.id)]));
  let n = 1;
  while (used.has(`${prefix}${n}`)) {
    n++;
  }
  return `${prefix}${n}`;
}

// optionValue makes the value of an option from its label: lower case, letters and digits.
export function optionValue(label: string, taken: Set<string>): string {
  const base =
    label
      .toLowerCase()
      .normalize("NFKD")
      .replace(/[^a-z0-9]+/g, "_")
      .replace(/^_|_$/g, "") || "option";
  let v = base;
  for (let n = 2; taken.has(v); n++) {
    v = `${base}_${n}`;
  }
  taken.add(v);
  return v;
}

type Lang = "de" | "en";
type Condition = NonNullable<FormQuestion["show_if"]>[number];

// Candidate is a question a condition may name: one before the element, with the values to
// compare with when they are fixed (yes/no, the options of a choice).
type Candidate = { id: string; name: string; values?: { value: string; label: string }[] };

// text is a text of the structure in the language, else German.
function text(label: Text | undefined, lang: string): string {
  return label?.[lang] || label?.["de"] || "";
}

export function FormBuilder({
  id,
  label,
  value,
  onChange,
  values,
  invalid,
  describedBy,
}: EditorProps) {
  const { t, i18n } = useTranslation();
  const id2 = useId();
  const [lang, setLang] = useState<Lang>("de");
  const types = useQuery(typesQuery);
  const target = types.data?.find((c) => qualifiedName(c) === String(values["target_type"] ?? ""));
  const { def, broken } = parse(value);
  const set = (next: FormDefinition) => onChange(JSON.stringify(next, null, 2));
  const setSection = (i: number, s: FormSection) =>
    set({ ...def, sections: def.sections.map((x, j) => (j === i ? s : x)) });
  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const bound = new Set(def.sections.flatMap((s) => s.questions.map((q) => q.field ?? "")));

  const questionName = (q: FormQuestion): string => {
    const own = text(q.label, i18n.language);
    if (own) {
      return own;
    }
    const f = q.field ? target?.fields.find((x) => x.name === q.field) : undefined;
    return f ? fieldLabel(f, i18n.language) : q.field || q.id;
  };
  const sectionName = (s: FormSection, i: number): string =>
    text(s.title, i18n.language) || t("builder.section", { n: i + 1 });
  // candidate describes a question for the conditions of later elements.
  const candidate = (q: FormQuestion): Candidate => {
    const f = q.field ? target?.fields.find((x) => x.name === q.field) : undefined;
    const typ = f ? f.type : q.type;
    const options = f ? f.options : q.options;
    let fixed: Candidate["values"];
    if (typ === "boolean") {
      fixed = [
        { value: "true", label: t("builder.yes") },
        { value: "false", label: t("builder.no") },
      ];
    } else if (typ === "choice") {
      fixed = (options ?? []).map((o) => ({
        value: o.value,
        label: labelOf(o.label, i18n.language, o.value) || o.value,
      }));
    }
    return { id: q.id, name: questionName(q), values: fixed };
  };
  // before are the candidates before each section, in order of the form.
  const before: Candidate[][] = [];
  {
    const seen: Candidate[] = [];
    for (const s of def.sections) {
      before.push([...seen]);
      seen.push(...s.questions.map(candidate));
    }
  }
  const nameOf = (dragId: string | number): string => {
    for (const [i, s] of def.sections.entries()) {
      if (s.id === dragId) {
        return sectionName(s, i);
      }
      const q = s.questions.find((x) => x.id === dragId);
      if (q) {
        return questionName(q);
      }
    }
    return String(dragId);
  };
  // dnd-kit reports the element over itself right after picking it up; that would replace
  // "picked up" at once, so only another element is announced.
  const announcements: Announcements = {
    onDragStart: ({ active }) => t("builder.dnd.start", { name: nameOf(active.id) }),
    onDragOver: ({ active, over }) =>
      over && over.id !== active.id
        ? t("builder.dnd.over", { name: nameOf(active.id), other: nameOf(over.id) })
        : undefined,
    onDragEnd: ({ active, over }) =>
      over && over.id !== active.id
        ? t("builder.dnd.end", { name: nameOf(active.id), other: nameOf(over.id) })
        : t("builder.dnd.stay", { name: nameOf(active.id) }),
    onDragCancel: ({ active }) => t("builder.dnd.cancel", { name: nameOf(active.id) }),
  };
  const accessibility = {
    announcements,
    screenReaderInstructions: { draggable: t("builder.dnd.help") },
  };

  const moveSection = (e: DragEndEvent) => {
    const from = def.sections.findIndex((s) => s.id === e.active.id);
    const to = def.sections.findIndex((s) => s.id === e.over?.id);
    if (from >= 0 && to >= 0 && from !== to) {
      set({ ...def, sections: arrayMove(def.sections, from, to) });
    }
  };

  return (
    <fieldset
      id={id}
      className="flex flex-col gap-4 rounded-md border p-4"
      aria-invalid={invalid}
      aria-describedby={describedBy}
    >
      <legend className="px-1 text-sm font-semibold">{label}</legend>
      {broken && <Alert variant="destructive">{t("builder.broken")}</Alert>}
      {!target && <Alert>{t("builder.noTarget")}</Alert>}
      <div role="radiogroup" aria-label={t("builder.language")} className="flex gap-4 text-sm">
        {(["de", "en"] as const).map((l) => (
          <label key={l} className="flex items-center gap-2">
            <input
              type="radio"
              name={`${id2}-lang`}
              checked={lang === l}
              onChange={() => setLang(l)}
            />
            {t(`builder.lang.${l}`)}
          </label>
        ))}
      </div>
      {lang === "en" && (
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id2}-title`}>{t("builder.titleEn")}</Label>
            <Input
              id={`${id2}-title`}
              value={def.title?.["en"] ?? ""}
              onChange={(e) => set({ ...def, title: { ...def.title, en: e.target.value } })}
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id2}-intro`}>{t("builder.introEn")}</Label>
            <Input
              id={`${id2}-intro`}
              value={def.intro?.["en"] ?? ""}
              onChange={(e) => set({ ...def, intro: { ...def.intro, en: e.target.value } })}
            />
          </div>
        </div>
      )}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={moveSection}
        accessibility={accessibility}
      >
        <SortableContext
          items={def.sections.map((s) => s.id)}
          strategy={verticalListSortingStrategy}
        >
          {def.sections.map((s, i) => (
            <SectionCard
              key={s.id}
              section={s}
              name={sectionName(s, i)}
              first={i === 0}
              last={i === def.sections.length - 1}
              onUp={() => set({ ...def, sections: arrayMove(def.sections, i, i - 1) })}
              onDown={() => set({ ...def, sections: arrayMove(def.sections, i, i + 1) })}
              onRemove={() => set({ ...def, sections: def.sections.filter((_, j) => j !== i) })}
            >
              <SectionBody
                section={s}
                lang={lang}
                before={before[i] ?? []}
                candidate={candidate}
                target={target}
                bound={bound}
                questionName={questionName}
                newId={(prefix) => nextId(def, prefix)}
                sensors={sensors}
                accessibility={accessibility}
                onChange={(next) => setSection(i, next)}
              />
            </SectionCard>
          ))}
        </SortableContext>
      </DndContext>
      <div>
        <Button
          variant="outline"
          onClick={() =>
            set({
              ...def,
              sections: [
                ...def.sections,
                { id: nextId(def, "s"), title: { de: "" }, questions: [] },
              ],
            })
          }
        >
          {t("builder.addSection")}
        </Button>
      </div>
    </fieldset>
  );
}

// Handle is the grip to drag an element with mouse or keyboard; the type editor uses it too.
export function Handle({
  name,
  listeners,
  attributes,
}: {
  name: string;
  listeners: ReturnType<typeof useSortable>["listeners"];
  attributes: ReturnType<typeof useSortable>["attributes"];
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className="cursor-grab rounded-md border px-2 py-1 text-sm focus-visible:outline focus-visible:outline-2"
      {...attributes}
      {...listeners}
      aria-label={t("builder.drag", { name })}
    >
      <span aria-hidden="true">⠿</span>
    </button>
  );
}

// MoveButtons are "up", "down" and "remove" for one element.
function MoveButtons({
  name,
  first,
  last,
  onUp,
  onDown,
  onRemove,
}: {
  name: string;
  first: boolean;
  last: boolean;
  onUp: () => void;
  onDown: () => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex gap-1">
      <Button
        variant="outline"
        size="sm"
        disabled={first}
        onClick={onUp}
        aria-label={t("builder.up", { name })}
      >
        ↑
      </Button>
      <Button
        variant="outline"
        size="sm"
        disabled={last}
        onClick={onDown}
        aria-label={t("builder.down", { name })}
      >
        ↓
      </Button>
      <Button
        variant="outline"
        size="sm"
        onClick={onRemove}
        aria-label={t("builder.remove", { name })}
      >
        ✕
      </Button>
    </div>
  );
}

function SectionCard({
  section,
  name,
  first,
  last,
  onUp,
  onDown,
  onRemove,
  children,
}: {
  section: FormSection;
  name: string;
  first: boolean;
  last: boolean;
  onUp: () => void;
  onDown: () => void;
  onRemove: () => void;
  children: ReactNode;
}) {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({
    id: section.id,
  });
  return (
    <section
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      aria-label={name}
      className="flex flex-col gap-3 rounded-md border bg-background p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Handle name={name} listeners={listeners} attributes={attributes} />
          <h2 className="font-semibold">{name}</h2>
        </div>
        <MoveButtons
          name={name}
          first={first}
          last={last}
          onUp={onUp}
          onDown={onDown}
          onRemove={onRemove}
        />
      </div>
      {children}
    </section>
  );
}

function SectionBody({
  section,
  lang,
  before,
  candidate,
  target,
  bound,
  questionName,
  newId,
  sensors,
  accessibility,
  onChange,
}: {
  section: FormSection;
  lang: Lang;
  before: Candidate[];
  candidate: (q: FormQuestion) => Candidate;
  target?: TypeDefinition;
  bound: Set<string>;
  questionName: (q: FormQuestion) => string;
  newId: (prefix: string) => string;
  sensors: ReturnType<typeof useSensors>;
  accessibility: { announcements: Announcements; screenReaderInstructions: { draggable: string } };
  onChange: (s: FormSection) => void;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const [pick, setPick] = useState("");
  const qs = section.questions;
  const setQuestions = (questions: FormQuestion[]) => onChange({ ...section, questions });
  const fields = (target?.fields ?? []).filter((f) => !isCalculated(f) && !bound.has(f.name));
  const add = () => {
    if (!pick) {
      return;
    }
    const [kind, name] = pick.split(":");
    const q: FormQuestion =
      kind === "field"
        ? { id: newId("q"), field: name }
        : { id: newId("q"), type: name as FormQuestion["type"], label: { de: "" } };
    setQuestions([...qs, q]);
    setPick("");
  };
  const move = (e: DragEndEvent) => {
    const from = qs.findIndex((q) => q.id === e.active.id);
    const to = qs.findIndex((q) => q.id === e.over?.id);
    if (from >= 0 && to >= 0 && from !== to) {
      setQuestions(arrayMove(qs, from, to));
    }
  };
  return (
    <>
      <div className="flex flex-col gap-1">
        <Label htmlFor={`${id}-title`}>{inLang(t("builder.sectionTitle"), lang, t)}</Label>
        <Input
          id={`${id}-title`}
          value={section.title[lang] ?? ""}
          placeholder={lang === "en" ? (section.title["de"] ?? "") : undefined}
          onChange={(e) =>
            onChange({ ...section, title: { ...section.title, [lang]: e.target.value } })
          }
        />
      </div>
      <Conditions
        list={section.show_if ?? []}
        candidates={before}
        onChange={(show_if) =>
          onChange({ ...section, show_if: show_if.length > 0 ? show_if : undefined })
        }
      />
      {qs.length === 0 && <p className="text-sm text-muted-foreground">{t("builder.empty")}</p>}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={move}
        accessibility={accessibility}
      >
        <SortableContext items={qs.map((q) => q.id)} strategy={verticalListSortingStrategy}>
          <ol className="flex flex-col gap-2">
            {qs.map((q, i) => (
              <QuestionRow
                key={q.id}
                question={q}
                lang={lang}
                before={[...before, ...qs.slice(0, i).map(candidate)]}
                name={questionName(q)}
                field={q.field ? target?.fields.find((f) => f.name === q.field) : undefined}
                first={i === 0}
                last={i === qs.length - 1}
                onUp={() => setQuestions(arrayMove(qs, i, i - 1))}
                onDown={() => setQuestions(arrayMove(qs, i, i + 1))}
                onRemove={() => setQuestions(qs.filter((_, j) => j !== i))}
                onChange={(next) => setQuestions(qs.map((x, j) => (j === i ? next : x)))}
              />
            ))}
          </ol>
        </SortableContext>
      </DndContext>
      <div className="flex flex-wrap items-end gap-2">
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-pick`}>{t("builder.newQuestion")}</Label>
          <select
            id={`${id}-pick`}
            className="h-10 rounded-md border border-input bg-background px-3 text-sm"
            value={pick}
            onChange={(e) => setPick(e.target.value)}
          >
            <option value="">{t("builder.choose")}</option>
            {fields.length > 0 && (
              <optgroup label={t("builder.fieldGroup")}>
                {fields.map((f) => (
                  <option key={f.name} value={`field:${f.name}`}>
                    {fieldLabel(f, i18n.language)}
                  </option>
                ))}
              </optgroup>
            )}
            <optgroup label={t("builder.freeGroup")}>
              {freeTypes.map((ft) => (
                <option key={ft} value={`free:${ft}`}>
                  {t(`builder.type.${ft}`)}
                </option>
              ))}
            </optgroup>
          </select>
        </div>
        <Button variant="outline" onClick={add} disabled={!pick}>
          {t("builder.addQuestion")}
        </Button>
      </div>
    </>
  );
}

function QuestionRow({
  question: q,
  lang,
  before,
  name,
  field,
  first,
  last,
  onUp,
  onDown,
  onRemove,
  onChange,
}: {
  question: FormQuestion;
  lang: Lang;
  before: Candidate[];
  name: string;
  field?: Field;
  first: boolean;
  last: boolean;
  onUp: () => void;
  onDown: () => void;
  onRemove: () => void;
  onChange: (q: FormQuestion) => void;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({ id: q.id });
  // The text of the options as typed: an empty line stays while one types the next.
  const [optionText, setOptionText] = useState(() =>
    (q.options ?? []).map((o) => o.label["de"] ?? o.value).join("\n"),
  );
  const kind = q.field
    ? t("builder.boundTo", { field: field ? fieldLabel(field, i18n.language) : q.field })
    : t(`builder.type.${q.type ?? "text"}`);
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      aria-label={name}
      className="flex flex-col gap-2 rounded-md border bg-background p-2"
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Handle name={name} listeners={listeners} attributes={attributes} />
          <span className="font-medium">{name || t("builder.unnamed")}</span>
          <span className="text-sm text-muted-foreground">{kind}</span>
        </div>
        <MoveButtons
          name={name}
          first={first}
          last={last}
          onUp={onUp}
          onDown={onDown}
          onRemove={onRemove}
        />
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex min-w-60 flex-1 flex-col gap-1">
          <Label htmlFor={`${id}-label`}>
            {inLang(q.field ? t("builder.labelOptional") : t("builder.label"), lang, t)}
          </Label>
          <Input
            id={`${id}-label`}
            value={q.label?.[lang] ?? ""}
            placeholder={lang === "en" ? (q.label?.["de"] ?? "") : undefined}
            onChange={(e) => onChange({ ...q, label: { ...q.label, [lang]: e.target.value } })}
          />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4"
            checked={q.required === true}
            onChange={(e) => onChange({ ...q, required: e.target.checked })}
          />
          {t("builder.required")}
        </label>
      </div>
      {q.type === "choice" && lang === "en" && (
        <div className="flex flex-col gap-2">
          {(q.options ?? []).map((o, k) => (
            <div key={o.value} className="flex flex-col gap-1">
              <Label htmlFor={`${id}-option-${k}`}>
                {t("builder.optionEn", { option: o.label["de"] ?? o.value })}
              </Label>
              <Input
                id={`${id}-option-${k}`}
                value={o.label["en"] ?? ""}
                onChange={(e) =>
                  onChange({
                    ...q,
                    options: (q.options ?? []).map((x, j) =>
                      j === k ? { ...x, label: { ...x.label, en: e.target.value } } : x,
                    ),
                  })
                }
              />
            </div>
          ))}
        </div>
      )}
      {q.type === "choice" && lang === "de" && (
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-options`}>{t("builder.options")}</Label>
          <textarea
            id={`${id}-options`}
            className="min-h-20 rounded-md border border-input bg-background p-2 text-sm"
            value={optionText}
            onChange={(e) => {
              setOptionText(e.target.value);
              const taken = new Set<string>();
              const old = new Map((q.options ?? []).map((o) => [o.label["de"] ?? "", o]));
              const options = e.target.value
                .split("\n")
                .map((line) => line.trim())
                .filter((line) => line !== "")
                .map((line) => {
                  // An option keeps its value and its English while its label stays the same.
                  const kept = old.get(line);
                  const value =
                    kept && !taken.has(kept.value) ? kept.value : optionValue(line, taken);
                  taken.add(value);
                  return { value, label: { ...kept?.label, de: line } };
                });
              onChange({ ...q, options });
            }}
          />
        </div>
      )}
      <Conditions
        list={q.show_if ?? []}
        candidates={before}
        onChange={(show_if) =>
          onChange({ ...q, show_if: show_if.length > 0 ? show_if : undefined })
        }
      />
    </li>
  );
}

// inLang marks the label of an input with the language being edited, when it is not German.
function inLang(label: string, lang: Lang, t: (key: string, o?: Record<string, string>) => string) {
  return lang === "de" ? label : t("builder.inEnglish", { label });
}

// Conditions edits "show only when" of a section or question (UI12): each rule names an
// earlier question and what its answer must be; all rules must hold.
function Conditions({
  list,
  candidates,
  onChange,
}: {
  list: Condition[];
  candidates: Candidate[];
  onChange: (list: Condition[]) => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const set = (k: number, c: Condition) => onChange(list.map((x, j) => (j === k ? c : x)));
  const first = candidates[0];
  return (
    <div className="flex flex-col gap-2">
      {list.length > 0 && (
        <fieldset className="flex flex-col gap-2 rounded-md border border-dashed p-2">
          <legend className="px-1 text-sm">{t("builder.cond.legend")}</legend>
          {list.map((c, k) => {
            const target = candidates.find((x) => x.id === c.question);
            const needsValue = c.op === "eq" || c.op === "ne";
            return (
              <div key={k} className="flex flex-wrap items-end gap-2">
                <div className="flex flex-col gap-1">
                  <Label htmlFor={`${id}-${k}-q`}>{t("builder.cond.question")}</Label>
                  <select
                    id={`${id}-${k}-q`}
                    className="h-10 rounded-md border border-input bg-background px-3 text-sm"
                    value={c.question}
                    onChange={(e) => set(k, { ...c, question: e.target.value, value: undefined })}
                  >
                    {!target && <option value={c.question}>{c.question}</option>}
                    {candidates.map((x) => (
                      <option key={x.id} value={x.id}>
                        {x.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="flex flex-col gap-1">
                  <Label htmlFor={`${id}-${k}-op`}>{t("builder.cond.op")}</Label>
                  <select
                    id={`${id}-${k}-op`}
                    className="h-10 rounded-md border border-input bg-background px-3 text-sm"
                    value={c.op}
                    onChange={(e) => {
                      const op = e.target.value as Condition["op"];
                      set(k, {
                        ...c,
                        op,
                        value: op === "eq" || op === "ne" ? c.value : undefined,
                      });
                    }}
                  >
                    {(["eq", "ne", "answered", "empty"] as const).map((op) => (
                      <option key={op} value={op}>
                        {t(`builder.cond.ops.${op}`)}
                      </option>
                    ))}
                  </select>
                </div>
                {needsValue && (
                  <div className="flex flex-col gap-1">
                    <Label htmlFor={`${id}-${k}-v`}>{t("builder.cond.value")}</Label>
                    {target?.values ? (
                      <select
                        id={`${id}-${k}-v`}
                        className="h-10 rounded-md border border-input bg-background px-3 text-sm"
                        value={c.value ?? ""}
                        onChange={(e) => set(k, { ...c, value: e.target.value })}
                      >
                        <option value="">{t("builder.choose")}</option>
                        {target.values.map((v) => (
                          <option key={v.value} value={v.value}>
                            {v.label}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <Input
                        id={`${id}-${k}-v`}
                        value={c.value ?? ""}
                        onChange={(e) => set(k, { ...c, value: e.target.value })}
                      />
                    )}
                  </div>
                )}
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onChange(list.filter((_, j) => j !== k))}
                  aria-label={t("builder.cond.remove", { n: String(k + 1) })}
                >
                  ✕
                </Button>
              </div>
            );
          })}
        </fieldset>
      )}
      <div>
        <Button
          variant="outline"
          size="sm"
          disabled={!first}
          onClick={() =>
            first &&
            onChange([
              ...list,
              first.values
                ? { question: first.id, op: "eq", value: first.values[0]?.value }
                : { question: first.id, op: "answered" },
            ])
          }
        >
          {t("builder.cond.add")}
        </Button>
      </div>
    </div>
  );
}
