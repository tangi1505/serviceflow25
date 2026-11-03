// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The type and field editor (UI25, E29): changes a type in the customer layer (PL43, Hauptplan
// E37, E135). Every action is one change, saved at once with the version of the layer seen;
// another version is someone else's change in between, and the editor loads afresh. Fields of
// their own (k_…) come in all seven field types and may be removed; fields of the package
// change in label, description, required and default, get options, are hidden or moved.
// Below the fields stand the changes with who, when and whether they apply, each to be taken
// back, and the export of the customer layer as a file. The overview lists the types with
// their number of changes.
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
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { useId, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  customerLayerQuery,
  labelOf,
  meQuery,
  qualifiedName,
  typesQuery,
  type CustomerChange,
  type CustomerLayer,
  type Field,
  type Label as Text,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";

import { Handle, optionValue } from "./FormBuilder";
import { fieldLabel, formatMoment, isCalculated } from "./fields";

// prefix marks the fields of the customer layer (Kern-Plan PL43).
const prefix = "k_";

// kinds are what a new field may be: the seven field types, text also as formatted text.
const kinds = [
  "text",
  "rich",
  "number",
  "boolean",
  "date",
  "datetime",
  "choice",
  "reference",
] as const;
// A running number (PL49) is shown, but not offered for a new field yet.
type Kind = (typeof kinds)[number] | "sequence";

const own = (f: Field) => f.name.startsWith(prefix);

// slug makes the technical name of a new field from its label.
function slug(label: string): string {
  const base = label
    .toLowerCase()
    .replace(/ä/g, "ae")
    .replace(/ö/g, "oe")
    .replace(/ü/g, "ue")
    .replace(/ß/g, "ss")
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_|_$/g, "");
  return (prefix + base).slice(0, 63);
}

// kindOf names the kind of a field as the editor shows it.
function kindOf(f: Field): Kind {
  return f.type === "text" && f.format === "rich" ? "rich" : f.type;
}

// save hands a Blob to the browser as a download.
function save(file: Blob, name: string) {
  const url = URL.createObjectURL(file);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// ExportButton downloads every customer layer as JSON, the content of a package of the
// customer (PL43).
function ExportButton() {
  const { t } = useTranslation();
  const [failure, setFailure] = useState<string | null>(null);
  const run = useMutation({
    mutationFn: api.customerLayers,
    onMutate: () => setFailure(null),
    onSuccess: (layers) =>
      save(
        new Blob([JSON.stringify(layers, null, 2)], { type: "application/json" }),
        "kundenschicht.json",
      ),
    onError: (err) => setFailure(err instanceof Error ? err.message : String(err)),
  });
  return (
    <div className="flex flex-col gap-2">
      <div>
        <Button variant="outline" disabled={run.isPending} onClick={() => run.mutate()}>
          {t("customize.export")}
        </Button>
      </div>
      {failure && <Alert variant="destructive">{t("customize.error", { detail: failure })}</Alert>}
    </div>
  );
}

// problemText turns a failed change into one message: the reasons of a 422, the conflict of a
// 409, the missing right of a 403. The role editor (UI27) uses it too.
export function useProblemText() {
  const { t } = useTranslation();
  return (err: unknown): string => {
    if (err instanceof ApiError) {
      if (err.status === 409) {
        return t("customize.conflict");
      }
      if (err.status === 403) {
        return t("customize.forbidden");
      }
      const reasons = err.fieldErrors.map((e) => e.message).filter(Boolean);
      if (reasons.length > 0) {
        return t("customize.refused", { detail: reasons.join("; ") });
      }
    }
    return t("customize.error", { detail: err instanceof Error ? err.message : String(err) });
  };
}

export type Apply = (c: CustomerChange, message: string) => void;

// useCustomizer is the editing of a customer layer: the layer, a change with the version
// seen, taking one back, and the outcome as a message. The type editor (UI25) and the
// workflow editor (UI26.1) share it.
export function useCustomizer(typeName: string) {
  const layer = useQuery(customerLayerQuery(typeName));
  const queryClient = useQueryClient();
  const problemText = useProblemText();
  const [failure, setFailure] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);

  // After a change the layer is the answer, and the type, lists and forms use the new
  // definition (its revision changed).
  const settle = async (next: CustomerLayer, message: string) => {
    queryClient.setQueryData(customerLayerQuery(typeName).queryKey, next);
    await queryClient.invalidateQueries({ queryKey: ["types"] });
    setDone(message);
  };
  const fail = async (err: unknown) => {
    setFailure(problemText(err));
    if (err instanceof ApiError && err.status === 409) {
      await queryClient.invalidateQueries({ queryKey: ["customer", typeName] });
      await queryClient.invalidateQueries({ queryKey: ["types"] });
    }
  };
  const change = useMutation({
    mutationFn: ({ c }: { c: CustomerChange; message: string }) =>
      api.addCustomerChange(typeName, layer.data?.version ?? 0, c),
    onMutate: () => {
      setFailure(null);
      setDone(null);
    },
    onSuccess: (next, { message }) => settle(next, message),
    onError: fail,
  });
  const takeBack = useMutation({
    mutationFn: ({ id }: { id: string; message: string }) =>
      api.removeCustomerChange(typeName, id, layer.data?.version ?? 0),
    onMutate: () => {
      setFailure(null);
      setDone(null);
    },
    onSuccess: (next, { message }) => settle(next, message),
    onError: fail,
  });
  const apply: Apply = (c, message) => change.mutate({ c, message });
  return {
    layer,
    apply,
    takeBack: (id: string, message: string) => takeBack.mutate({ id, message }),
    busy: change.isPending || takeBack.isPending,
    failure,
    done,
    problemText,
  };
}

export function CustomizePage() {
  const { t, i18n } = useTranslation();
  const { typeName } = useParams({ from: "/shell/types/$typeName/customize" });
  const types = useQuery(typesQuery);
  const type = types.data?.find((c) => qualifiedName(c) === typeName);
  const ed = useCustomizer(typeName);

  if (types.isPending || ed.layer.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  if (ed.layer.isError) {
    return <Alert variant="destructive">{ed.problemText(ed.layer.error)}</Alert>;
  }
  const typeLabel = labelOf(type.label, i18n.language, typeName);
  return (
    <div className="flex max-w-4xl flex-col gap-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t("customize.title", { type: typeLabel })}</h1>
        <div className="flex gap-4">
          {/* The workflow editor (UI26.1) edits states and transitions of the same type. */}
          <Link to="/types/$typeName/workflow" params={{ typeName }} className="text-sm underline">
            {t("workflow.open")}
          </Link>
          <Link to="/types/$typeName" params={{ typeName }} className="text-sm underline">
            {t("customize.toList")}
          </Link>
        </div>
      </div>
      <p className="text-sm text-muted-foreground">{t("customize.intro")}</p>
      {ed.failure && <Alert variant="destructive">{ed.failure}</Alert>}
      {ed.done && <Alert variant="success">{ed.done}</Alert>}
      <Fields type={type} busy={ed.busy} apply={ed.apply} />
      <NewField type={type} busy={ed.busy} apply={ed.apply} />
      <Changes type={type} layer={ed.layer.data} busy={ed.busy} onTakeBack={ed.takeBack} />
    </div>
  );
}

// Fields lists the fields of the effective type in their order, to be dragged, moved, hidden,
// edited and, if the customer layer added them, removed.
function Fields({ type, busy, apply }: { type: TypeDefinition; busy: boolean; apply: Apply }) {
  const { t, i18n } = useTranslation();
  const fields = type.fields;
  const name = (f: Field) => fieldLabel(f, i18n.language);
  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const nameOf = (id: string | number) => {
    const f = fields.find((x) => x.name === id);
    return f ? name(f) : String(id);
  };
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
  // move puts the field from index from to index to: behind the field before that place.
  const move = (from: number, to: number) => {
    if (from === to || to < 0 || to >= fields.length) {
      return;
    }
    const next = arrayMove(fields, from, to);
    const f = fields[from]!;
    const after = to > 0 ? next[to - 1]!.name : "";
    apply(
      { op: "move_field", field: f.name, after },
      after
        ? t("customize.done.moved", { name: name(f), after: nameOf(after) })
        : t("customize.done.movedFirst", { name: name(f) }),
    );
  };
  const onDragEnd = (e: DragEndEvent) =>
    move(
      fields.findIndex((f) => f.name === e.active.id),
      fields.findIndex((f) => f.name === e.over?.id),
    );
  return (
    <section aria-labelledby="customize-fields" className="flex flex-col gap-3">
      <h2 id="customize-fields" className="text-lg font-semibold">
        {t("customize.fields")}
      </h2>
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={onDragEnd}
        accessibility={{
          announcements,
          screenReaderInstructions: { draggable: t("builder.dnd.help") },
        }}
      >
        <SortableContext items={fields.map((f) => f.name)} strategy={verticalListSortingStrategy}>
          <ul className="flex flex-col gap-2">
            {fields.map((f, i) => (
              <FieldRow
                key={f.name}
                type={type}
                field={f}
                first={i === 0}
                last={i === fields.length - 1}
                busy={busy}
                apply={apply}
                onUp={() => move(i, i - 1)}
                onDown={() => move(i, i + 1)}
              />
            ))}
          </ul>
        </SortableContext>
      </DndContext>
    </section>
  );
}

function FieldRow({
  type,
  field,
  first,
  last,
  busy,
  apply,
  onUp,
  onDown,
}: {
  type: TypeDefinition;
  field: Field;
  first: boolean;
  last: boolean;
  busy: boolean;
  apply: Apply;
  onUp: () => void;
  onDown: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [editing, setEditing] = useState(false);
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({
    id: field.name,
  });
  const name = fieldLabel(field, i18n.language);
  const flags = [
    own(field) ? t("customize.flag.own") : t("customize.flag.package"),
    field.required ? t("customize.flag.required") : "",
    field.hidden ? t("customize.flag.hidden") : "",
    isCalculated(field) ? t("customize.flag.calculated") : "",
    field.name === type.title_field ? t("customize.flag.title") : "",
  ].filter(Boolean);
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      aria-label={name}
      className="flex flex-col gap-3 rounded-md border bg-background p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Handle name={name} listeners={listeners} attributes={attributes} />
          <div className="flex flex-col">
            <span className={field.hidden ? "font-medium text-muted-foreground" : "font-medium"}>
              {name}
            </span>
            <span className="text-xs text-muted-foreground">
              {field.name} · {t(`customize.kind.${kindOf(field)}`)} · {flags.join(" · ")}
            </span>
          </div>
        </div>
        <div className="flex flex-wrap gap-1">
          <Button
            variant="outline"
            size="sm"
            disabled={busy || first}
            onClick={onUp}
            aria-label={t("builder.up", { name })}
          >
            ↑
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={busy || last}
            onClick={onDown}
            aria-label={t("builder.down", { name })}
          >
            ↓
          </Button>
          <Button
            variant="outline"
            size="sm"
            aria-expanded={editing}
            onClick={() => setEditing(!editing)}
            aria-label={t("customize.edit", { name })}
          >
            {t("customize.editShort")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() =>
              apply(
                { op: field.hidden ? "show_field" : "hide_field", field: field.name },
                t(field.hidden ? "customize.done.shown" : "customize.done.hidden", { name }),
              )
            }
            aria-label={t(field.hidden ? "customize.show" : "customize.hide", { name })}
          >
            {t(field.hidden ? "customize.showShort" : "customize.hideShort")}
          </Button>
          {own(field) && (
            <Button
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() =>
                apply(
                  { op: "remove_field", field: field.name },
                  t("customize.done.removed", { name }),
                )
              }
              aria-label={t("customize.remove", { name })}
            >
              {t("customize.removeShort")}
            </Button>
          )}
        </div>
      </div>
      {editing && (
        <FieldEditor field={field} busy={busy} apply={apply} onClose={() => setEditing(false)} />
      )}
    </li>
  );
}

// defaultText shows a default for the input; parseDefault reads it back for the field type.
function defaultText(v: unknown): string {
  return v === undefined || v === null ? "" : String(v);
}

function parseDefault(f: Field, raw: string): unknown {
  if (raw === "") {
    return undefined;
  }
  if (f.type === "number") {
    return Number(raw);
  }
  if (f.type === "boolean") {
    return raw === "true";
  }
  return raw;
}

// FieldEditor changes label, description, required, default and, on fields of its own, the
// maximum length; a choice field gets more options. Only what differs is sent.
function FieldEditor({
  field,
  busy,
  apply,
  onClose,
}: {
  field: Field;
  busy: boolean;
  apply: Apply;
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const name = fieldLabel(field, i18n.language);
  const [labelDe, setLabelDe] = useState(field.label?.["de"] ?? "");
  const [labelEn, setLabelEn] = useState(field.label?.["en"] ?? "");
  const [descDe, setDescDe] = useState(field.description?.["de"] ?? "");
  const [descEn, setDescEn] = useState(field.description?.["en"] ?? "");
  const [required, setRequired] = useState(field.required === true);
  const [def, setDef] = useState(defaultText(field.default));
  const [maxLength, setMaxLength] = useState(String(field.max_length ?? ""));
  const [options, setOptions] = useState("");
  const textual = field.type === "text" && field.format !== "rich";
  const withDefault =
    ["text", "number", "boolean", "choice"].includes(field.type) && !isCalculated(field);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const set: Record<string, unknown> = {};
    const label: Text = {};
    if (labelDe.trim() && labelDe !== (field.label?.["de"] ?? "")) {
      label["de"] = labelDe.trim();
    }
    if (labelEn.trim() && labelEn !== (field.label?.["en"] ?? "")) {
      label["en"] = labelEn.trim();
    }
    if (Object.keys(label).length > 0) {
      set["label"] = label;
    }
    const description: Text = {};
    if (descDe.trim() && descDe !== (field.description?.["de"] ?? "")) {
      description["de"] = descDe.trim();
    }
    if (descEn.trim() && descEn !== (field.description?.["en"] ?? "")) {
      description["en"] = descEn.trim();
    }
    if (Object.keys(description).length > 0) {
      set["description"] = description;
    }
    if (required !== (field.required === true)) {
      set["required"] = required;
    }
    if (withDefault && def !== defaultText(field.default) && def !== "") {
      set["default"] = parseDefault(field, def);
    }
    if (own(field) && textual && maxLength !== String(field.max_length ?? "") && maxLength !== "") {
      set["max_length"] = Number(maxLength);
    }
    if (Object.keys(set).length > 0) {
      // The message names the field as it is called after the change.
      apply(
        { op: "change_field", field: field.name, set },
        t("customize.done.changed", { name: label["de"] ?? name }),
      );
      onClose();
    }
  };
  // addOptions is a change of its own: one change at a time keeps the version of the layer.
  const addOptions = () => {
    const taken = new Set((field.options ?? []).map((o) => o.value));
    const added = options
      .split("\n")
      .map((l) => l.trim())
      .filter(Boolean)
      .map((l) => ({ value: optionValue(l, taken), label: { de: l } }));
    if (added.length > 0) {
      apply(
        { op: "add_options", field: field.name, options: added },
        t("customize.done.options", { name, options: added.map((o) => o.label.de).join(", ") }),
      );
      onClose();
    }
  };

  return (
    <form className="flex flex-col gap-3 border-t pt-3" onSubmit={submit}>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-de`}>{t("customize.labelDe")}</Label>
          <Input id={`${id}-de`} value={labelDe} onChange={(e) => setLabelDe(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-en`}>{t("customize.labelEn")}</Label>
          <Input id={`${id}-en`} value={labelEn} onChange={(e) => setLabelEn(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-dde`}>{t("customize.descDe")}</Label>
          <Input id={`${id}-dde`} value={descDe} onChange={(e) => setDescDe(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-den`}>{t("customize.descEn")}</Label>
          <Input id={`${id}-den`} value={descEn} onChange={(e) => setDescEn(e.target.value)} />
        </div>
        {withDefault && (
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-def`}>{t("customize.default")}</Label>
            {field.type === "choice" || field.type === "boolean" ? (
              <select
                id={`${id}-def`}
                className="h-10 rounded-md border border-input bg-background px-3 text-sm"
                value={def}
                onChange={(e) => setDef(e.target.value)}
              >
                <option value="">{t("customize.noDefault")}</option>
                {field.type === "boolean" ? (
                  <>
                    <option value="true">{t("builder.yes")}</option>
                    <option value="false">{t("builder.no")}</option>
                  </>
                ) : (
                  (field.options ?? []).map((o) => (
                    <option key={o.value} value={o.value}>
                      {labelOf(o.label, i18n.language, o.value)}
                    </option>
                  ))
                )}
              </select>
            ) : (
              <Input
                id={`${id}-def`}
                type={field.type === "number" ? "number" : "text"}
                value={def}
                onChange={(e) => setDef(e.target.value)}
              />
            )}
          </div>
        )}
        {own(field) && textual && (
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-max`}>{t("customize.maxLength")}</Label>
            <Input
              id={`${id}-max`}
              type="number"
              min={1}
              value={maxLength}
              onChange={(e) => setMaxLength(e.target.value)}
            />
          </div>
        )}
      </div>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={required} onChange={(e) => setRequired(e.target.checked)} />
        {t("customize.required")}
      </label>
      {field.type === "choice" && (
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-opts`}>{t("customize.moreOptions")}</Label>
          <textarea
            id={`${id}-opts`}
            className="min-h-20 rounded-md border border-input bg-background px-3 py-2 text-sm"
            value={options}
            onChange={(e) => setOptions(e.target.value)}
          />
          <div>
            <Button variant="outline" disabled={busy || options.trim() === ""} onClick={addOptions}>
              {t("customize.addOptions")}
            </Button>
          </div>
        </div>
      )}
      <div className="flex gap-2">
        <Button type="submit" disabled={busy}>
          {t("customize.save")}
        </Button>
        <Button variant="outline" onClick={onClose}>
          {t("form.cancel")}
        </Button>
      </div>
    </form>
  );
}

// NewField adds a field of its own; the technical name follows the label until it is typed.
function NewField({ type, busy, apply }: { type: TypeDefinition; busy: boolean; apply: Apply }) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const types = useQuery(typesQuery);
  const [labelDe, setLabelDe] = useState("");
  const [labelEn, setLabelEn] = useState("");
  const [technical, setTechnical] = useState<string | null>(null);
  const [kind, setKind] = useState<Kind>("text");
  const [required, setRequired] = useState(false);
  const [maxLength, setMaxLength] = useState("200");
  const [options, setOptions] = useState("");
  const [to, setTo] = useState("");
  const fieldName = technical ?? slug(labelDe);
  const nameOk = /^k_[a-z][a-z0-9_]{0,60}$/.test(fieldName);
  const lines = options
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean);
  const ready =
    labelDe.trim() !== "" &&
    nameOk &&
    (kind !== "choice" || lines.length > 0) &&
    (kind !== "reference" || to !== "");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!ready) {
      return;
    }
    const label: Text = { de: labelDe.trim() };
    if (labelEn.trim()) {
      label["en"] = labelEn.trim();
    }
    const field: Record<string, unknown> = {
      name: fieldName,
      type: kind === "rich" ? "text" : kind,
      label,
    };
    if (required) {
      field["required"] = true;
    }
    if (kind === "rich") {
      field["format"] = "rich";
    }
    if (kind === "text" && maxLength !== "") {
      field["max_length"] = Number(maxLength);
    }
    if (kind === "choice") {
      const taken = new Set<string>();
      field["options"] = lines.map((l) => ({ value: optionValue(l, taken), label: { de: l } }));
    }
    if (kind === "reference") {
      field["to"] = to;
    }
    apply(
      { op: "add_field", field: fieldName, new: field },
      t("customize.done.added", { name: labelDe.trim() }),
    );
    setLabelDe("");
    setLabelEn("");
    setTechnical(null);
    setRequired(false);
    setOptions("");
  };

  return (
    <section aria-labelledby={`${id}-h`} className="flex flex-col gap-3">
      <h2 id={`${id}-h`} className="text-lg font-semibold">
        {t("customize.newField")}
      </h2>
      <form className="flex flex-col gap-3 rounded-md border p-3" onSubmit={submit}>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-de`}>{t("customize.labelDe")}</Label>
            <Input id={`${id}-de`} value={labelDe} onChange={(e) => setLabelDe(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-en`}>{t("customize.labelEn")}</Label>
            <Input id={`${id}-en`} value={labelEn} onChange={(e) => setLabelEn(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-name`}>{t("customize.technical")}</Label>
            <Input
              id={`${id}-name`}
              value={fieldName}
              aria-invalid={labelDe !== "" && !nameOk}
              aria-describedby={`${id}-name-help`}
              onChange={(e) => setTechnical(e.target.value)}
            />
            <p id={`${id}-name-help`} className="text-xs text-muted-foreground">
              {t("customize.technicalHelp")}
            </p>
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-kind`}>{t("customize.kindLabel")}</Label>
            <select
              id={`${id}-kind`}
              className="h-10 rounded-md border border-input bg-background px-3 text-sm"
              value={kind}
              onChange={(e) => setKind(e.target.value as Kind)}
            >
              {kinds.map((k) => (
                <option key={k} value={k}>
                  {t(`customize.kind.${k}`)}
                </option>
              ))}
            </select>
          </div>
          {kind === "text" && (
            <div className="flex flex-col gap-1">
              <Label htmlFor={`${id}-max`}>{t("customize.maxLength")}</Label>
              <Input
                id={`${id}-max`}
                type="number"
                min={1}
                value={maxLength}
                onChange={(e) => setMaxLength(e.target.value)}
              />
            </div>
          )}
          {kind === "reference" && (
            <div className="flex flex-col gap-1">
              <Label htmlFor={`${id}-to`}>{t("customize.to")}</Label>
              <select
                id={`${id}-to`}
                className="h-10 rounded-md border border-input bg-background px-3 text-sm"
                value={to}
                onChange={(e) => setTo(e.target.value)}
              >
                <option value="">{t("builder.choose")}</option>
                {(types.data ?? []).map((c) => (
                  <option key={qualifiedName(c)} value={qualifiedName(c)}>
                    {labelOf(c.label, i18n.language, qualifiedName(c))}
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>
        {kind === "choice" && (
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-opts`}>{t("customize.options")}</Label>
            <textarea
              id={`${id}-opts`}
              className="min-h-20 rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={options}
              onChange={(e) => setOptions(e.target.value)}
            />
          </div>
        )}
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={required}
            onChange={(e) => setRequired(e.target.checked)}
          />
          {t("customize.required")}
        </label>
        <div>
          <Button type="submit" disabled={busy || !ready}>
            {t("customize.add", { type: labelOf(type.label, i18n.language, type.name) })}
          </Button>
        </div>
      </form>
    </section>
  );
}

// describe says in words what a change does, with the labels of the fields, states and
// transitions as they are now.
function useDescribe(type: TypeDefinition) {
  const { t, i18n } = useTranslation();
  const nameOf = (field: string) => {
    const f = type.fields.find((x) => x.name === field);
    if (f) {
      return fieldLabel(f, i18n.language);
    }
    const s = type.states?.find((x) => x.name === field);
    if (s) {
      return labelOf(s.label, i18n.language, s.name);
    }
    const tr = type.transitions?.find((x) => x.name === field);
    return tr ? labelOf(tr.label, i18n.language, tr.name) : field;
  };
  const what = (set: Record<string, unknown> | undefined) =>
    Object.keys(set ?? {})
      .map((k) => t(`customize.attr.${k}`, { defaultValue: k }))
      .join(", ");
  return (c: CustomerChange): string => {
    const name = nameOf(c.field);
    switch (c.op) {
      case "add_field": {
        const label = labelOf(c.new?.["label"] as Text | undefined, i18n.language, c.field);
        return t("customize.op.add_field", { name: label, field: c.field });
      }
      case "add_state": {
        const label = labelOf(c.state?.["label"] as Text | undefined, i18n.language, c.field);
        return t("customize.op.add_state", { name: label, field: c.field });
      }
      case "add_transition": {
        const label = labelOf(c.transition?.["label"] as Text | undefined, i18n.language, c.field);
        return t("customize.op.add_transition", { name: label, field: c.field });
      }
      case "change_field":
      case "change_state":
      case "change_transition":
        return t(`customize.op.${c.op}`, { name, what: what(c.set) });
      case "add_options":
        return t("customize.op.add_options", {
          name,
          options: (c.options ?? [])
            .map((o) => labelOf(o["label"] as Text | undefined, i18n.language, String(o["value"])))
            .join(", "),
        });
      case "move_field":
        return c.after
          ? t("customize.op.move_field", { name, after: nameOf(c.after) })
          : t("customize.op.move_first", { name });
      default:
        return t(`customize.op.${c.op}`, { name });
    }
  };
}

// Changes lists the customer layer, newest first, each with who, when and whether it applies;
// only narrows it to the changes of one editor.
export function Changes({
  type,
  layer,
  busy,
  only,
  onTakeBack,
}: {
  type: TypeDefinition;
  layer: CustomerLayer;
  busy: boolean;
  only?: (c: CustomerChange) => boolean;
  onTakeBack: (id: string, message: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const describe = useDescribe(type);
  const changes = [...layer.changes].reverse().filter(only ?? (() => true));
  return (
    <section aria-labelledby="customize-changes" className="flex flex-col gap-3">
      <h2 id="customize-changes" className="text-lg font-semibold">
        {t("customize.changes")}
      </h2>
      {changes.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("customize.noChanges")}</p>
      ) : (
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b">
              <th scope="col" className="py-2 pr-3">
                {t("customize.col.change")}
              </th>
              <th scope="col" className="py-2 pr-3">
                {t("customize.col.by")}
              </th>
              <th scope="col" className="py-2 pr-3">
                {t("customize.col.at")}
              </th>
              <th scope="col" className="py-2 pr-3">
                {t("customize.col.status")}
              </th>
              <th scope="col" className="py-2">
                <span className="sr-only">{t("customize.col.action")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {changes.map((c) => {
              const text = describe(c);
              return (
                <tr key={c.id} className="border-b align-top">
                  <td className="py-2 pr-3">{text}</td>
                  <td className="py-2 pr-3">{c.by_name ?? "—"}</td>
                  <td className="py-2 pr-3">{c.at ? formatMoment(c.at, i18n.language) : ""}</td>
                  <td className="py-2 pr-3">
                    <span className={c.status === "active" ? "" : "font-medium text-destructive"}>
                      {t(`customize.status.${c.status ?? "active"}`)}
                    </span>
                    {c.reason && <div className="text-xs text-muted-foreground">{c.reason}</div>}
                  </td>
                  <td className="py-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busy || !c.id}
                      onClick={() =>
                        onTakeBack(c.id!, t("customize.done.takenBack", { change: text }))
                      }
                      aria-label={t("customize.takeBack", { change: text })}
                    >
                      {t("customize.takeBackShort")}
                    </Button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      <ExportButton />
    </section>
  );
}

// CustomizeOverview lists the types with their number of changes in the customer layer and
// links to their editors (Verwaltung › Typen anpassen).
export function CustomizeOverview() {
  const { t, i18n } = useTranslation();
  const types = useQuery(typesQuery);
  const me = useQuery(meQuery);
  const layers = useQuery({
    queryKey: ["customer"],
    queryFn: api.customerLayers,
    enabled: me.data?.customize === true,
  });
  if (me.data && me.data.customize !== true) {
    return <Alert variant="destructive">{t("customize.forbidden")}</Alert>;
  }
  if (types.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  const count = (name: string) => layers.data?.find((l) => l.type === name)?.changes.length ?? 0;
  return (
    <div className="flex max-w-3xl flex-col gap-4">
      <h1 className="text-2xl font-semibold">{t("customize.overview")}</h1>
      <p className="text-sm text-muted-foreground">{t("customize.overviewIntro")}</p>
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b">
            <th scope="col" className="py-2 pr-3">
              {t("customize.col.type")}
            </th>
            <th scope="col" className="py-2">
              {t("customize.col.count")}
            </th>
          </tr>
        </thead>
        <tbody>
          {(types.data ?? []).map((c) => {
            const name = qualifiedName(c);
            return (
              <tr key={name} className="border-b">
                <td className="py-2 pr-3">
                  <Link
                    to="/types/$typeName/customize"
                    params={{ typeName: name }}
                    className="underline"
                  >
                    {labelOf(c.label, i18n.language, name)}
                  </Link>
                  <span className="ml-2 text-xs text-muted-foreground">{name}</span>
                </td>
                <td className="py-2">{count(name)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
      <ExportButton />
    </div>
  );
}
