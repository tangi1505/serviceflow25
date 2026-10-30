// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The form of a record, rendered from the fields of its type (react-hook-form + zod). Saving
// sends If-Match with the version seen; a 409 means someone saved in between: the form shows
// both values of every differing field and lets the user reload or overwrite (Hauptplan E29).
// The field states of the record's state and variant (WF2) hide fields, make them required or
// read-only, preset a new record and lock a record as a whole; the server checks them again.
// References and the roles at the record are chosen with the picker (UI28); the roles are a
// section of the form and saved with the fields, only those that changed. A field can have an
// editor of its own (the structure of a form, UI11); a form of the portal groups its questions
// in sections and sends them its own way (UI11).
import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useId, useState, type ReactNode } from "react";
import { Controller, get, useForm, useWatch } from "react-hook-form";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  labelOf,
  qualifiedName,
  type ActorRef,
  type Field,
  type FieldStates,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";

import {
  changedFields,
  fieldLabel,
  formatValue,
  isCalculated,
  optionLabel,
  schemaFor,
  toInput,
  toRecord,
} from "./fields";
import { FormBuilder } from "./FormBuilder";
import { RefPicker } from "./RefPicker";
import { RichEditor, RichText } from "./RichEditor";

type Conflict = { theirs: RecordData; mine: RecordData; fields: Field[] };

// EditorProps are what an editor of its own gets: its value, the other values of the form
// (the builder reads the target type), and how the form labels and checks it.
export type EditorProps = {
  id: string;
  label: string;
  value: unknown;
  onChange: (value: unknown) => void;
  values: Record<string, unknown>;
  invalid?: boolean;
  describedBy?: string;
};

// editors are the fields with an editor of their own, by type and field.
const editors: Record<string, (p: EditorProps) => ReactNode> = {
  "core.form.definition": (p) => <FormBuilder {...p} />,
};
type Roles = Record<string, ActorRef[]>;

// rolesOf reads the roles of a record ({role: [{type, id}]}); a new one has none.
function rolesOf(record: RecordData | undefined): Roles {
  const raw = (record?.["actors"] ?? {}) as Record<string, ActorRef[] | null>;
  const out: Roles = {};
  for (const [name, list] of Object.entries(raw)) {
    out[name] = (list ?? []).map((r) => ({ type: r.type, id: r.id }));
  }
  return out;
}

// changedRoles are the roles whose holders differ from the record's, as the API takes them:
// a role not named stays as it is, so a session without the right to write roles can still
// save the fields.
function changedRoles(before: Roles, now: Roles): Roles | undefined {
  const key = (l: ActorRef[] | undefined) =>
    (l ?? [])
      .map((r) => `${r.type}:${r.id}`)
      .sort()
      .join(",");
  const out: Roles = {};
  for (const [name, list] of Object.entries(now)) {
    if (key(list) !== key(before[name])) {
      out[name] = list;
    }
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

export type FormProps = {
  type: TypeDefinition;
  // record is the loaded record when editing; undefined creates.
  record?: RecordData;
  // states are the field states for the record or the new one (WF2); undefined means none.
  states?: FieldStates;
  onSaved: (saved: RecordData) => void;
  // presets are the person's values for a new record (UI9); field states win over them.
  presets?: Record<string, unknown>;
  // The portal (UI10): only these fields, no section of roles, and extra values sent with a
  // new record (the requester).
  only?: string[];
  withRoles?: boolean;
  extra?: Record<string, unknown>;
  // onDirtyChange says whether the form holds unsaved changes (UI8: presence, live reload).
  onDirtyChange?: (dirty: boolean) => void;
  // A form of the portal (UI11): its sections, each a legend over the fields it names, and
  // where a new record goes instead of the API of the type.
  groups?: { key: string; title: string; fields: string[] }[];
  submit?: (body: RecordData) => Promise<RecordData>;
  // visibility names the fields and groups hidden for the values (the conditions of a form,
  // UI12): hidden ones are not shown, not checked and not sent.
  visibility?: (values: Record<string, unknown>) => { fields: Set<string>; groups: Set<string> };
  onCancel: () => void;
};

// shownType is the type as the form shows it: hidden fields left out, required ones marked,
// presets as defaults.
function shownType(
  type: TypeDefinition,
  states?: FieldStates,
  presets?: Record<string, unknown>,
): TypeDefinition {
  if (presets) {
    // The person's presets (UI9) before the defaults of the type.
    type = {
      ...type,
      fields: type.fields.map((f) => (f.name in presets ? { ...f, default: presets[f.name] } : f)),
    };
  }
  if (!states) {
    return type;
  }
  const hidden = new Set(states.hidden);
  const required = new Set(states.required);
  return {
    ...type,
    fields: type.fields
      .filter((f) => !hidden.has(f.name))
      .map((f) => ({
        ...f,
        required: f.required === true || required.has(f.name),
        default: f.name in states.defaults ? states.defaults[f.name] : f.default,
      })),
  };
}

export function Form({
  type: fullType,
  record,
  states,
  onSaved,
  onCancel,
  onDirtyChange,
  presets,
  only,
  withRoles = true,
  extra,
  groups,
  submit,
  visibility,
}: FormProps) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const typeName = qualifiedName(fullType);
  // Fields the customer layer hid (PL43) are not in the form; the server keeps their values.
  const visibleType = { ...fullType, fields: fullType.fields.filter((f) => !f.hidden) };
  const shown = shownType(visibleType, states, record ? undefined : presets);
  const type = only
    ? { ...shown, fields: shown.fields.filter((f) => only.includes(f.name)) }
    : shown;
  const locked = states?.locked === true;
  const readOnly = new Set(states?.read_only ?? []);
  const [base, setBase] = useState<RecordData | undefined>(record);
  const [conflict, setConflict] = useState<Conflict | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const [roles, setRoles] = useState<Roles>(() => rolesOf(record));
  const {
    control: formControl,
    register,
    handleSubmit,
    reset,
    setError,
    formState: { errors, isSubmitting, isDirty },
  } = useForm<Record<string, unknown>>({
    // Read-only fields are not checked: the form does not send them; nor hidden ones (UI12),
    // which follow from the values being checked.
    resolver: (vals, context, options) => {
      const hidden = visibility?.(vals).fields;
      const checked = type.fields.filter((f) => !readOnly.has(f.name) && !hidden?.has(f.name));
      return zodResolver(schemaFor({ ...type, fields: checked }))(vals, context, options);
    },
    defaultValues: toInput(type, record),
  });
  const values = useWatch({ control: formControl });
  const hidden = visibility?.({ ...toInput(type, record), ...values });

  const dirty = isDirty || changedRoles(rolesOf(base), roles) !== undefined;
  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);

  const save = async (values: Record<string, unknown>, version?: number) => {
    setFailure(null);
    setConflict(null);
    const body = toRecord(type, values);
    for (const name of readOnly) {
      // Read-only in this state: the server keeps it, the form does not send it.
      delete body[name];
    }
    for (const name of visibility?.(values).fields ?? []) {
      // Hidden by a condition (UI12): not an answer.
      delete body[name];
    }
    const roleChanges = readOnly.has("actors") ? undefined : changedRoles(rolesOf(base), roles);
    if (roleChanges) {
      body["actors"] = roleChanges;
    }
    if (!base && extra) {
      Object.assign(body, extra);
    }
    try {
      const saved = base
        ? await api.update(typeName, String(base["id"]), version ?? Number(base["version"]), body)
        : submit
          ? await submit(body)
          : await api.create(typeName, body);
      onSaved(saved);
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        for (const e of err.fieldErrors) {
          setError(e.field, { message: e.message });
        }
        if (err.fieldErrors.length === 0) {
          setFailure(err.message);
        }
        return;
      }
      if (err instanceof ApiError && err.status === 409 && base) {
        const theirs = await api.get(typeName, String(base["id"]));
        setConflict({ theirs, mine: body, fields: changedFields(type, theirs, body) });
        return;
      }
      setFailure(t("form.error", { detail: err instanceof Error ? err.message : String(err) }));
    }
  };

  const reload = () => {
    if (conflict) {
      setBase(conflict.theirs);
      reset(toInput(type, conflict.theirs));
      setRoles(rolesOf(conflict.theirs));
      setConflict(null);
    }
  };

  const overwrite = () => {
    if (conflict) {
      setBase(conflict.theirs);
      void save(
        toInput(type, { ...conflict.theirs, ...conflict.mine }),
        Number(conflict.theirs["version"]),
      );
    }
  };

  const errorText = (name: string) => {
    // A role's problem is named "actors.<role>" and lands nested in errors.
    const raw = (get(errors, name) as { message?: unknown } | undefined)?.message;
    if (!raw) {
      return null;
    }
    const code = String(raw);
    const known = ["required", "number", "min", "max", "max_length", "choice", "uuid", "read_only"];
    return (
      <p id={`${id}-${name}-error`} className="text-sm text-destructive">
        {known.includes(code) ? t(`form.rule.${code}`) : code}
      </p>
    );
  };

  const control = (f: Field) => {
    const common = {
      id: `${id}-${f.name}`,
      "aria-invalid": errors[f.name] ? true : undefined,
      "aria-describedby": errors[f.name] ? `${id}-${f.name}-error` : undefined,
      "aria-required": f.required ? true : undefined,
    };
    const editor = editors[`${typeName}.${f.name}`];
    if (editor && !readOnly.has(f.name)) {
      return (
        <Controller
          name={f.name}
          control={formControl}
          render={({ field }) => (
            <>
              {editor({
                id: common.id,
                label: fieldLabel(f, i18n.language),
                value: field.value,
                onChange: field.onChange,
                values,
                invalid: common["aria-invalid"],
                describedBy: common["aria-describedby"],
              })}
            </>
          )}
        />
      );
    }
    if (isCalculated(f)) {
      // Sums of the costs (PL20) and formulas (PL22): kept by the server, shown read-only.
      return <Input readOnly {...common} {...register(f.name)} />;
    }
    if (readOnly.has(f.name) && f.format === "rich") {
      // Formatted text (UI7), read-only: the HTML the server cleaned.
      return <RichText html={String(record?.[f.name] ?? "")} className="rounded-md border p-2" />;
    }
    if (readOnly.has(f.name)) {
      // Read-only in this state (WF2): shown as text, never sent.
      return (
        <Input
          readOnly
          {...common}
          value={formatValue(f, record?.[f.name] ?? f.default, i18n.language)}
        />
      );
    }
    switch (f.type) {
      case "boolean":
        return <input type="checkbox" className="h-4 w-4" {...common} {...register(f.name)} />;
      case "number":
        return (
          <Input
            type="number"
            step="any"
            min={f.min}
            max={f.max}
            {...common}
            {...register(f.name, { valueAsNumber: true })}
          />
        );
      case "date":
        return <Input type="date" {...common} {...register(f.name)} />;
      case "datetime":
        return <Input type="datetime-local" {...common} {...register(f.name)} />;
      case "text":
        if (f.format !== "rich") {
          return <Input maxLength={f.max_length || undefined} {...common} {...register(f.name)} />;
        }
        // Formatted text (UI7): the editor; the server sanitizes the HTML.
        return (
          <Controller
            name={f.name}
            control={formControl}
            render={({ field }) => (
              <RichEditor
                id={common.id}
                label={fieldLabel(f, i18n.language)}
                describedBy={common["aria-describedby"]}
                invalid={common["aria-invalid"]}
                value={String(field.value ?? "")}
                onChange={field.onChange}
              />
            )}
          />
        );
      case "reference":
        // One record of the type the field points to, chosen by name (UI28).
        return (
          <Controller
            name={f.name}
            control={formControl}
            render={({ field }) => (
              <RefPicker
                id={common.id}
                targets={f.to ? [f.to] : []}
                single
                required={f.required}
                invalid={common["aria-invalid"]}
                describedBy={common["aria-describedby"]}
                value={field.value ? [{ type: f.to ?? "", id: String(field.value) }] : []}
                onChange={(refs) => field.onChange(refs[0]?.id ?? "")}
              />
            )}
          />
        );
      case "choice":
        return (
          <select
            className="h-10 rounded-md border border-input bg-background px-3 text-sm"
            {...common}
            {...register(f.name)}
          >
            {!f.required && <option value="">{t("form.none")}</option>}
            {(f.options ?? []).map((o) => (
              <option key={o.value} value={o.value}>
                {optionLabel(f, o.value, i18n.language)}
              </option>
            ))}
          </select>
        );
      default:
        return <Input maxLength={f.max_length || undefined} {...common} {...register(f.name)} />;
    }
  };

  const row = (f: Field) => {
    if (editors[`${typeName}.${f.name}`] && !readOnly.has(f.name)) {
      // An editor of its own labels itself (a group with a legend).
      return (
        <div key={f.name} className="flex flex-col gap-2">
          {control(f)}
          {errorText(f.name)}
        </div>
      );
    }
    return (
      <div key={f.name} className="flex flex-col gap-2">
        <Label htmlFor={`${id}-${f.name}`}>
          {fieldLabel(f, i18n.language)}
          {f.required && <span aria-hidden="true"> *</span>}
        </Label>
        {groups && f.description && (
          <p className="text-sm text-muted-foreground">
            {labelOf(f.description, i18n.language, "")}
          </p>
        )}
        {control(f)}
        {errorText(f.name)}
      </div>
    );
  };

  return (
    <form
      className="flex max-w-2xl flex-col gap-4"
      onSubmit={handleSubmit((v) => save(v))}
      noValidate
    >
      {groups
        ? groups
            .filter((g) => !hidden?.groups.has(g.key))
            .map((g) => (
              <fieldset key={g.key} className="flex flex-col gap-4 rounded-md border p-4">
                <legend className="px-1 text-sm font-semibold">{g.title}</legend>
                {type.fields
                  .filter((f) => g.fields.includes(f.name) && !hidden?.fields.has(f.name))
                  .map(row)}
              </fieldset>
            ))
        : type.fields.filter((f) => !hidden?.fields.has(f.name)).map(row)}
      {withRoles && (fullType.actors ?? []).length > 0 && (
        <fieldset className="flex flex-col gap-4 rounded-md border p-4">
          <legend className="px-1 text-sm font-semibold">{t("form.roles")}</legend>
          {(fullType.actors ?? []).map((r) => (
            <div key={r.name} className="flex flex-col gap-2">
              <Label htmlFor={`${id}-role-${r.name}`}>
                {labelOf(r.label, i18n.language, r.name)}
              </Label>
              <RefPicker
                id={`${id}-role-${r.name}`}
                targets={r.to}
                single={r.single}
                disabled={locked || readOnly.has("actors")}
                value={roles[r.name] ?? []}
                onChange={(refs) => setRoles({ ...roles, [r.name]: refs })}
              />
              {errorText(`actors.${r.name}`)}
            </div>
          ))}
          {errorText("actors")}
        </fieldset>
      )}
      {conflict && (
        <Alert variant="destructive">
          <p className="font-medium">{t("form.conflict.title")}</p>
          <p>{t("form.conflict.text", { version: conflict.theirs["version"] })}</p>
          <table className="mt-2 w-full text-left text-sm">
            <thead>
              <tr>
                <th scope="col" className="pr-2">
                  {t("form.conflict.field")}
                </th>
                <th scope="col" className="pr-2">
                  {t("form.conflict.theirs")}
                </th>
                <th scope="col">{t("form.conflict.mine")}</th>
              </tr>
            </thead>
            <tbody>
              {conflict.fields.map((f) => (
                <tr key={f.name}>
                  <th scope="row" className="pr-2 font-normal">
                    {fieldLabel(f, i18n.language)}
                  </th>
                  <td className="pr-2">{formatValue(f, conflict.theirs[f.name], i18n.language)}</td>
                  <td>{formatValue(f, conflict.mine[f.name], i18n.language)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="mt-3 flex gap-2">
            <Button variant="outline" size="sm" onClick={reload}>
              {t("form.conflict.reload")}
            </Button>
            <Button variant="destructive" size="sm" onClick={overwrite}>
              {t("form.conflict.overwrite")}
            </Button>
          </div>
        </Alert>
      )}
      {failure && <Alert variant="destructive">{failure}</Alert>}
      {locked && (
        <Alert>
          {t("form.locked", {
            state: labelOf(
              fullType.states?.find((s) => s.name === states?.state)?.label,
              i18n.language,
              states?.state ?? "",
            ),
          })}
        </Alert>
      )}
      <div className="flex gap-2">
        {!locked && (
          <Button type="submit" disabled={isSubmitting}>
            {base ? t("form.save") : submit ? t("form.send") : t("form.create")}
          </Button>
        )}
        <Button variant="outline" onClick={onCancel}>
          {t("form.cancel")}
        </Button>
      </div>
    </form>
  );
}
